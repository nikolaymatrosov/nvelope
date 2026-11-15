package adapters

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"
	"github.com/ydb-platform/ydb-go-sdk/v3/table/types"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// YDBTracking is the YDB-backed implementation of domain.TrackingRepository,
// for the serverless runtime. The two resolvers read through the global
// indexes idx_links_id and idx_campaigns_id, which stand in for the Postgres
// SECURITY DEFINER functions.
type YDBTracking struct {
	driver *ydb.Driver
}

var _ domain.TrackingRepository = (*YDBTracking)(nil)

// NewYDBTracking builds a YDBTracking repository over the driver.
func NewYDBTracking(d *ydb.Driver) *YDBTracking {
	return &YDBTracking{driver: d}
}

// UpsertLinks ensures one links row per distinct URL and returns the map from
// URL to its links-row id. It reads the URLs the campaign already has and
// inserts the others, in one transaction: an INSERT onto an existing key would
// abort it. A concurrent upsert of the same URL aborts one of the two
// transactions, and the SDK retries it.
func (r *YDBTracking) UpsertLinks(ctx context.Context, tenantID, campaignID string,
	urls []string) (map[string]string, error) {

	seen := make(map[string]bool, len(urls))
	var distinct []string
	for _, u := range urls {
		if !seen[u] {
			seen[u] = true
			distinct = append(distinct, u)
		}
	}
	out := make(map[string]string, len(distinct))
	if len(distinct) == 0 {
		return out, nil
	}

	// Ids are fixed before the transaction, so a retry writes the same rows.
	fresh := make(map[string]string, len(distinct))
	values := make([]types.Value, len(distinct))
	for i, u := range distinct {
		fresh[u] = uuid.NewString()
		values[i] = types.TextValue(u)
	}

	var result map[string]string
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		rs, err := tx.QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $campaign_id AS Utf8;
DECLARE $urls AS List<Utf8>;
SELECT url, id FROM links VIEW idx_links_url
WHERE tenant_id = $tenant_id AND campaign_id = $campaign_id AND url IN $urls;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$campaign_id").Text(campaignID).
				Param("$urls").Any(types.ListValue(values...)).
				Build()))
		if err != nil {
			return fmt.Errorf("reading links: %w", err)
		}
		// Built in a local and assigned at the end (V6).
		local := make(map[string]string, len(distinct))
		for row, err := range rs.Rows(ctx) {
			if err != nil {
				return fmt.Errorf("reading links: %w", err)
			}
			var url, id string
			if err := row.Scan(&url, &id); err != nil {
				return fmt.Errorf("scanning link: %w", err)
			}
			local[url] = id
		}
		var rows []types.Value
		for _, u := range distinct {
			if _, ok := local[u]; ok {
				continue
			}
			local[u] = fresh[u]
			rows = append(rows, types.StructValue(
				types.StructFieldValue("id", types.TextValue(fresh[u])),
				types.StructFieldValue("url", types.TextValue(u))))
		}
		if len(rows) > 0 {
			err = tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $campaign_id AS Utf8;
DECLARE $rows AS List<Struct<id: Utf8, url: Utf8>>;
INSERT INTO links (tenant_id, id, campaign_id, url, created_at)
SELECT $tenant_id, id, $campaign_id, url, CurrentUtcTimestamp()
FROM AS_TABLE($rows);`, query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$campaign_id").Text(campaignID).
				Param("$rows").Any(types.ListValue(rows...)).
				Build()))
			if err != nil {
				return fmt.Errorf("inserting links: %w", err)
			}
		}
		result = local
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ownRecipient reports whether the recipient exists in the tenant and belongs
// to the campaign. A missing or foreign id is false, never an error.
func ownRecipient(ctx context.Context, tx query.TxActor, tenantID, campaignID, recipientID string) (bool, error) {
	row, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT campaign_id FROM campaign_recipients VIEW idx_campaign_recipients_id
WHERE tenant_id = $tenant_id AND id = $id;`, ydbTenantAndID(tenantID, recipientID))
	if errors.Is(err, query.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("loading recipient: %w", err)
	}
	var owner string
	if err := row.Scan(&owner); err != nil {
		return false, fmt.Errorf("scanning recipient: %w", err)
	}
	return owner == campaignID, nil
}

// RecordClick returns the link's original URL and records the click when the
// recipient belongs to the link's campaign. A missing, malformed or foreign
// recipient id records nothing. An unknown link is domain.ErrLinkNotFound with
// an empty URL. When the insert itself fails, the URL is returned alongside
// the error so the caller can still redirect.
func (r *YDBTracking) RecordClick(ctx context.Context, tenantID, linkID, recipientID string) (string, error) {
	clickID := uuid.NewString()
	var url string
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		row, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT campaign_id, url FROM links WHERE tenant_id = $tenant_id AND id = $id;`,
			ydbTenantAndID(tenantID, linkID))
		if errors.Is(err, query.ErrNoRows) {
			return domain.ErrLinkNotFound
		}
		if err != nil {
			return fmt.Errorf("loading link: %w", err)
		}
		var campaignID, linkURL string
		if err := row.Scan(&campaignID, &linkURL); err != nil {
			return fmt.Errorf("scanning link: %w", err)
		}
		url = linkURL // the last run wins
		own, err := ownRecipient(ctx, tx, tenantID, campaignID, recipientID)
		if err != nil || !own {
			return err
		}
		err = tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $link_id AS Utf8;
DECLARE $campaign_id AS Utf8;
DECLARE $recipient_id AS Utf8;
INSERT INTO link_clicks (tenant_id, id, link_id, campaign_id, recipient_id, clicked_at)
VALUES ($tenant_id, $id, $link_id, $campaign_id, $recipient_id, CurrentUtcTimestamp());`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$id").Text(clickID).
				Param("$link_id").Text(linkID).
				Param("$campaign_id").Text(campaignID).
				Param("$recipient_id").Text(recipientID).
				Build()))
		if err != nil {
			return fmt.Errorf("recording click: %w", err)
		}
		return nil
	})
	return url, err
}

// RecordView records an open when the recipient belongs to the campaign. A
// missing or foreign recipient id records nothing.
func (r *YDBTracking) RecordView(ctx context.Context, tenantID, campaignID, recipientID string) error {
	viewID := uuid.NewString()
	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		own, err := ownRecipient(ctx, tx, tenantID, campaignID, recipientID)
		if err != nil || !own {
			return err
		}
		err = tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $campaign_id AS Utf8;
DECLARE $recipient_id AS Utf8;
INSERT INTO campaign_views (tenant_id, id, campaign_id, recipient_id, viewed_at)
VALUES ($tenant_id, $id, $campaign_id, $recipient_id, CurrentUtcTimestamp());`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$id").Text(viewID).
				Param("$campaign_id").Text(campaignID).
				Param("$recipient_id").Text(recipientID).
				Build()))
		if err != nil {
			return fmt.Errorf("recording view: %w", err)
		}
		return nil
	})
}

// ResolveTenantForLink returns the tenant that owns a links row.
func (r *YDBTracking) ResolveTenantForLink(ctx context.Context, linkID string) (string, error) {
	return r.resolveTenant(ctx, `DECLARE $id AS Utf8;
SELECT tenant_id FROM links VIEW idx_links_id WHERE id = $id;`, linkID, domain.ErrLinkNotFound)
}

// ResolveTenantForCampaign returns the tenant that owns a campaign.
func (r *YDBTracking) ResolveTenantForCampaign(ctx context.Context, campaignID string) (string, error) {
	return r.resolveTenant(ctx, `DECLARE $id AS Utf8;
SELECT tenant_id FROM campaigns VIEW idx_campaigns_id WHERE id = $id;`, campaignID, domain.ErrCampaignNotFound)
}

func (r *YDBTracking) resolveTenant(ctx context.Context, q, id string, notFound error) (string, error) {
	row, err := r.driver.Query().QueryRow(ctx, q,
		query.WithParameters(ydb.ParamsBuilder().Param("$id").Text(id).Build()))
	if errors.Is(err, query.ErrNoRows) {
		return "", notFound
	}
	if err != nil {
		return "", fmt.Errorf("resolving tenant: %w", err)
	}
	var tenantID string
	if err := row.Scan(&tenantID); err != nil {
		return "", fmt.Errorf("scanning tenant: %w", err)
	}
	return tenantID, nil
}

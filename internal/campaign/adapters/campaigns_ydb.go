package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// YDBCampaigns is the YDB-backed implementation of domain.CampaignRepository,
// for the serverless runtime. YDB has no row-level security, so every
// statement filters by tenant_id explicitly.
type YDBCampaigns struct {
	driver *ydb.Driver
}

var _ domain.CampaignRepository = (*YDBCampaigns)(nil)

// NewYDBCampaigns builds a YDBCampaigns repository over the driver.
func NewYDBCampaigns(d *ydb.Driver) *YDBCampaigns {
	return &YDBCampaigns{driver: d}
}

// ydbCampaignColumns is the projection scanYDBCampaign reads, in order.
const ydbCampaignColumns = `id, tenant_id, name, subject, body_html, body_text, from_name,
    from_local_part, sending_domain_id, template_id, status, max_send_errors,
    sent_count, failed_count, recipient_count, body_doc, theme,
    created_at, updated_at, started_at, finished_at, archive_visible, archived_at`

const ydbSelectCampaign = `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT ` + ydbCampaignColumns + `
FROM campaigns
WHERE tenant_id = $tenant_id AND id = $id;`

// ydbCampaignWrite declares the parameters Update binds from a campaign.
const ydbCampaignWrite = `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $name AS Utf8;
DECLARE $subject AS Utf8;
DECLARE $body_html AS Utf8;
DECLARE $body_text AS Utf8;
DECLARE $from_name AS Utf8;
DECLARE $from_local_part AS Utf8;
DECLARE $sending_domain_id AS Optional<Utf8>;
DECLARE $status AS Utf8;
DECLARE $max_send_errors AS Int64;
DECLARE $sent_count AS Int64;
DECLARE $failed_count AS Int64;
DECLARE $recipient_count AS Int64;
DECLARE $started_at AS Optional<Timestamp>;
DECLARE $finished_at AS Optional<Timestamp>;
DECLARE $archive_visible AS Bool;
DECLARE $archived_at AS Optional<Timestamp>;
DECLARE $body_doc AS Optional<Utf8>;
DECLARE $theme AS Optional<Utf8>;
`

// ydbCampaignParams binds every column Update writes. withTemplate adds
// template_id for Add: Update never changes it, as the Postgres UPDATE leaves
// it alone.
func ydbCampaignParams(tenantID, id string, c *domain.Campaign, withTemplate bool) query.ExecuteOption {
	b := ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$id").Text(id).
		Param("$name").Text(c.Name()).
		Param("$subject").Text(c.Subject()).
		Param("$body_html").Text(c.BodyHTML()).
		Param("$body_text").Text(c.BodyText()).
		Param("$from_name").Text(c.FromName()).
		Param("$from_local_part").Text(c.FromLocalPart()).
		Param("$sending_domain_id").BeginOptional().Text(nullableString(c.SendingDomainID())).EndOptional().
		Param("$status").Text(string(c.Status())).
		Param("$max_send_errors").Int64(int64(c.MaxSendErrors())).
		Param("$sent_count").Int64(int64(c.SentCount())).
		Param("$failed_count").Int64(int64(c.FailedCount())).
		Param("$recipient_count").Int64(int64(c.RecipientCount())).
		Param("$started_at").BeginOptional().Timestamp(c.StartedAt()).EndOptional().
		Param("$finished_at").BeginOptional().Timestamp(c.FinishedAt()).EndOptional().
		Param("$archive_visible").Bool(c.ArchiveVisible()).
		Param("$archived_at").BeginOptional().Timestamp(c.ArchivedAt()).EndOptional().
		Param("$body_doc").BeginOptional().Text(nullableJSONString(c.BodyDocJSON())).EndOptional().
		Param("$theme").BeginOptional().Text(nullableJSONString(c.ThemeJSON())).EndOptional()
	if withTemplate {
		b = b.Param("$template_id").BeginOptional().Text(nullableString(c.TemplateID())).EndOptional()
	}
	return query.WithParameters(b.Build())
}

// Add persists a new campaign under a fresh UUID and returns the id.
func (r *YDBCampaigns) Add(ctx context.Context, tenantID string, c *domain.Campaign) (string, error) {
	id := uuid.NewString()
	err := r.driver.Query().Exec(ctx, ydbCampaignWrite+`DECLARE $template_id AS Optional<Utf8>;
INSERT INTO campaigns
    (tenant_id, id, name, subject, body_html, body_text, from_name, from_local_part,
     sending_domain_id, template_id, status, max_send_errors, sent_count, failed_count,
     recipient_count, body_doc, theme, created_at, updated_at, started_at, finished_at,
     archive_visible, archived_at)
VALUES ($tenant_id, $id, $name, $subject, $body_html, $body_text, $from_name, $from_local_part,
    $sending_domain_id, $template_id, $status, $max_send_errors, $sent_count, $failed_count,
    $recipient_count, $body_doc, $theme, CurrentUtcTimestamp(), CurrentUtcTimestamp(),
    $started_at, $finished_at, $archive_visible, $archived_at);`, ydbCampaignParams(tenantID, id, c, true))
	if err != nil {
		return "", fmt.Errorf("inserting campaign: %w", err)
	}
	return id, nil
}

// Get returns the campaign, or domain.ErrCampaignNotFound.
func (r *YDBCampaigns) Get(ctx context.Context, tenantID, id string) (*domain.Campaign, error) {
	return ydbCampaignFromRow(r.driver.Query().QueryRow(ctx, ydbSelectCampaign, ydbTenantAndID(tenantID, id)))
}

// Update loads the campaign, runs fn, and persists the result in one
// serializable transaction. The SDK retries the transaction on conflict, so fn
// may run more than once and must be retry-safe.
func (r *YDBCampaigns) Update(ctx context.Context, tenantID, id string,
	fn func(*domain.Campaign) (*domain.Campaign, error)) error {

	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		loaded, err := ydbCampaignFromRow(tx.QueryRow(ctx, ydbSelectCampaign, ydbTenantAndID(tenantID, id)))
		if err != nil {
			return err
		}
		updated, err := fn(loaded)
		if err != nil {
			return err
		}
		err = tx.Exec(ctx, ydbCampaignWrite+`UPDATE campaigns SET
    name = $name, subject = $subject, body_html = $body_html, body_text = $body_text,
    from_name = $from_name, from_local_part = $from_local_part,
    sending_domain_id = $sending_domain_id, status = $status,
    max_send_errors = $max_send_errors, sent_count = $sent_count,
    failed_count = $failed_count, recipient_count = $recipient_count,
    started_at = $started_at, finished_at = $finished_at,
    archive_visible = $archive_visible, archived_at = $archived_at,
    body_doc = $body_doc, theme = $theme, updated_at = CurrentUtcTimestamp()
WHERE tenant_id = $tenant_id AND id = $id;`, ydbCampaignParams(tenantID, id, updated, false))
		if err != nil {
			return fmt.Errorf("updating campaign: %w", err)
		}
		return nil
	})
}

// All returns a page of the tenant's campaigns, newest first, and the total
// count, read in one transaction.
func (r *YDBCampaigns) All(ctx context.Context, tenantID string, page domain.Page) ([]*domain.Campaign, int, error) {
	return r.page(ctx, "campaigns", "", "created_at DESC", tenantID, page)
}

// Archived returns a page of the tenant's archive-visible campaigns,
// newest-first by archived_at, and the total count.
func (r *YDBCampaigns) Archived(ctx context.Context, tenantID string, page domain.Page) ([]*domain.Campaign, int, error) {
	return r.page(ctx, "archived campaigns", " AND archive_visible = true", "archived_at DESC", tenantID, page)
}

// page runs a COUNT(*) query and a page query over the tenant's campaigns in
// one transaction. where is an extra predicate, order the ORDER BY.
// ponytail: no index serves the archive filter or the order, so both scan the
// tenant's campaigns, and COUNT(*) plus OFFSET reads every matching row (spec
// "Ceilings"). Fine for one organisation's campaign history; keyset paging and
// an index on (tenant_id, archive_visible, archived_at) are the upgrade path.
func (r *YDBCampaigns) page(ctx context.Context, what, where, order, tenantID string,
	page domain.Page) ([]*domain.Campaign, int, error) {

	page = page.Normalize()
	var campaigns []*domain.Campaign
	var total int
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		count, err := ydbCount(ctx, tx, `DECLARE $tenant_id AS Utf8;
SELECT COUNT(*) FROM campaigns WHERE tenant_id = $tenant_id`+where+`;`, ydbTenantParam(tenantID))
		if err != nil {
			return fmt.Errorf("counting %s: %w", what, err)
		}
		rs, err := tx.QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $limit AS Uint64;
DECLARE $offset AS Uint64;
SELECT `+ydbCampaignColumns+`
FROM campaigns
WHERE tenant_id = $tenant_id`+where+`
ORDER BY `+order+`
LIMIT $limit OFFSET $offset;`, ydbTenantAndPage(tenantID, page.Limit, page.Offset))
		if err != nil {
			return fmt.Errorf("listing %s: %w", what, err)
		}
		// Collected into a local and assigned at the end: the SDK may re-run
		// the closure, and a retry must not append to rows a failed run read.
		var local []*domain.Campaign
		for row, err := range rs.Rows(ctx) {
			if err != nil {
				return fmt.Errorf("listing %s: %w", what, err)
			}
			c, err := scanYDBCampaign(row)
			if err != nil {
				return err
			}
			local = append(local, c)
		}
		campaigns, total = local, count
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return campaigns, total, nil
}

// SaveTargets replaces a campaign's targeted lists and segments. Every target
// gets a fresh UUID, and a list targeted twice collides on the unique index,
// as it does on Postgres.
func (r *YDBCampaigns) SaveTargets(ctx context.Context, tenantID, campaignID string,
	targets []domain.Target) error {

	ids := make([]string, len(targets))
	for i := range ids {
		ids[i] = uuid.NewString()
	}
	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $campaign_id AS Utf8;
DELETE FROM campaign_lists WHERE tenant_id = $tenant_id AND campaign_id = $campaign_id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$campaign_id").Text(campaignID).
				Build())); err != nil {
			return fmt.Errorf("clearing campaign targets: %w", err)
		}
		for i, t := range targets {
			err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $campaign_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $list_id AS Optional<Utf8>;
DECLARE $segment_query AS Optional<Utf8>;
INSERT INTO campaign_lists (tenant_id, campaign_id, id, list_id, segment_query)
VALUES ($tenant_id, $campaign_id, $id, $list_id, $segment_query);`,
				query.WithParameters(ydb.ParamsBuilder().
					Param("$tenant_id").Text(tenantID).
					Param("$campaign_id").Text(campaignID).
					Param("$id").Text(ids[i]).
					Param("$list_id").BeginOptional().Text(nullableString(t.ListID)).EndOptional().
					Param("$segment_query").BeginOptional().Text(nullableJSONString(t.SegmentQuery)).EndOptional().
					Build()))
			if err != nil {
				return fmt.Errorf("inserting campaign target: %w", err)
			}
		}
		return nil
	})
}

// Targets returns a campaign's targeted lists and segments.
func (r *YDBCampaigns) Targets(ctx context.Context, tenantID, campaignID string) ([]domain.Target, error) {
	var targets []domain.Target
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		rs, err := tx.QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $campaign_id AS Utf8;
SELECT list_id, segment_query FROM campaign_lists
WHERE tenant_id = $tenant_id AND campaign_id = $campaign_id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$campaign_id").Text(campaignID).
				Build()))
		if err != nil {
			return fmt.Errorf("listing campaign targets: %w", err)
		}
		// Collected into a local and assigned at the end (V6).
		var local []domain.Target
		for row, err := range rs.Rows(ctx) {
			if err != nil {
				return fmt.Errorf("listing campaign targets: %w", err)
			}
			var listID, segment *string
			if err := row.Scan(&listID, &segment); err != nil {
				return fmt.Errorf("scanning campaign target: %w", err)
			}
			local = append(local, domain.Target{ListID: deref(listID), SegmentQuery: jsonBytes(segment)})
		}
		targets = local
		return nil
	})
	if err != nil {
		return nil, err
	}
	return targets, nil
}

// ydbCampaignFromRow maps a single-row read to the campaign, translating "no
// row" into domain.ErrCampaignNotFound.
func ydbCampaignFromRow(row query.Row, err error) (*domain.Campaign, error) {
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrCampaignNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading campaign: %w", err)
	}
	return scanYDBCampaign(row)
}

// scanYDBCampaign reads one row in ydbCampaignColumns order.
func scanYDBCampaign(row query.Row) (*domain.Campaign, error) {
	var id, tenantID, name, subject, bodyHTML, bodyText, fromName, fromLocalPart, status string
	var sendingDomainID, templateID, bodyDoc, theme *string
	var maxSendErrors, sentCount, failedCount, recipientCount int64
	var createdAt, updatedAt time.Time
	var startedAt, finishedAt, archivedAt *time.Time
	var archiveVisible bool
	if err := row.Scan(&id, &tenantID, &name, &subject, &bodyHTML, &bodyText, &fromName,
		&fromLocalPart, &sendingDomainID, &templateID, &status, &maxSendErrors,
		&sentCount, &failedCount, &recipientCount, &bodyDoc, &theme,
		&createdAt, &updatedAt, &startedAt, &finishedAt, &archiveVisible, &archivedAt); err != nil {
		return nil, fmt.Errorf("scanning campaign: %w", err)
	}
	return domain.HydrateCampaign(id, tenantID, name, subject, bodyHTML, bodyText, fromName,
		fromLocalPart, deref(sendingDomainID), deref(templateID), domain.CampaignStatus(status),
		int(maxSendErrors), int(sentCount), int(failedCount), int(recipientCount),
		jsonBytes(bodyDoc), jsonBytes(theme),
		createdAt, updatedAt, startedAt, finishedAt, archiveVisible, archivedAt), nil
}

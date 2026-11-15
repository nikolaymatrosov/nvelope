package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
)

// YDBAnalytics is the YDB-backed implementation of domain.AnalyticsRepository,
// for the serverless runtime. Every query filters by the tenant's primary-key
// prefix.
type YDBAnalytics struct {
	driver *ydb.Driver
}

var _ domain.AnalyticsRepository = (*YDBAnalytics)(nil)

// NewYDBAnalytics builds a YDBAnalytics repository over the driver.
func NewYDBAnalytics(d *ydb.Driver) *YDBAnalytics {
	return &YDBAnalytics{driver: d}
}

// GetCampaign returns one campaign's pre-computed roll-up; ok is false when the
// campaign has no analytics row yet.
func (r *YDBAnalytics) GetCampaign(ctx context.Context, tenantID, campaignID string) (
	domain.CampaignAnalytics, bool, error) {

	row, err := r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $campaign_id AS Utf8;
SELECT sent_count, delivered_count, opened_count, clicked_count, bounced_count,
       complained_count, refreshed_at
FROM campaign_analytics WHERE tenant_id = $tenant_id AND campaign_id = $campaign_id;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$campaign_id").Text(campaignID).
			Build()))
	if errors.Is(err, query.ErrNoRows) {
		return domain.CampaignAnalytics{}, false, nil
	}
	if err != nil {
		return domain.CampaignAnalytics{}, false, fmt.Errorf("loading campaign analytics: %w", err)
	}
	var sent, delivered, opened, clicked, bounced, complained int64
	var refreshedAt time.Time
	if err := row.Scan(&sent, &delivered, &opened, &clicked, &bounced, &complained, &refreshedAt); err != nil {
		return domain.CampaignAnalytics{}, false, fmt.Errorf("scanning campaign analytics: %w", err)
	}
	return domain.CampaignAnalytics{
		CampaignID:  campaignID,
		Counts:      ydbCounts(sent, delivered, opened, clicked, bounced, complained),
		RefreshedAt: refreshedAt,
	}, true, nil
}

// ydbCounts maps the six Int64 count columns to domain.Counts.
func ydbCounts(sent, delivered, opened, clicked, bounced, complained int64) domain.Counts {
	return domain.Counts{
		Sent: int(sent), Delivered: int(delivered), Opened: int(opened),
		Clicked: int(clicked), Bounced: int(bounced), Complained: int(complained),
	}
}

// GetDashboard returns the tenant totals and the most recently sent campaigns.
// The totals and the list are read in one transaction, so they agree.
func (r *YDBAnalytics) GetDashboard(ctx context.Context, tenantID string) (domain.Dashboard, error) {
	var dash domain.Dashboard
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		row, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
SELECT COALESCE(SUM(sent_count), 0), COALESCE(SUM(delivered_count), 0),
       COALESCE(SUM(opened_count), 0), COALESCE(SUM(clicked_count), 0),
       COALESCE(SUM(bounced_count), 0), COALESCE(SUM(complained_count), 0)
FROM campaign_analytics WHERE tenant_id = $tenant_id;`, ydbTenantParam(tenantID))
		if err != nil {
			return fmt.Errorf("aggregating dashboard totals: %w", err)
		}
		var sent, delivered, opened, clicked, bounced, complained int64
		if err := row.Scan(&sent, &delivered, &opened, &clicked, &bounced, &complained); err != nil {
			return fmt.Errorf("scanning dashboard totals: %w", err)
		}

		rs, err := tx.QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
SELECT ca.campaign_id, c.name, ca.sent_count, ca.delivered_count, ca.opened_count,
       ca.clicked_count, ca.bounced_count, ca.complained_count
FROM campaign_analytics AS ca
JOIN campaigns AS c ON c.tenant_id = ca.tenant_id AND c.id = ca.campaign_id
WHERE ca.tenant_id = $tenant_id
ORDER BY COALESCE(c.started_at, c.created_at) DESC
LIMIT 10;`, ydbTenantParam(tenantID))
		if err != nil {
			return fmt.Errorf("listing recent campaigns: %w", err)
		}
		// Built in a local and assigned at the end (V6).
		var recent []domain.RecentCampaign
		for row, err := range rs.Rows(ctx) {
			if err != nil {
				return fmt.Errorf("listing recent campaigns: %w", err)
			}
			var rc domain.RecentCampaign
			var s, d, o, c, b, cm int64
			if err := row.Scan(&rc.CampaignID, &rc.Name, &s, &d, &o, &c, &b, &cm); err != nil {
				return fmt.Errorf("scanning recent campaign: %w", err)
			}
			rc.Counts = ydbCounts(s, d, o, c, b, cm)
			recent = append(recent, rc)
		}
		dash = domain.Dashboard{
			Totals: ydbCounts(sent, delivered, opened, clicked, bounced, complained),
			Recent: recent,
		}
		return nil
	})
	if err != nil {
		return domain.Dashboard{}, err
	}
	return dash, nil
}

// Refresh recomputes every campaign_analytics row of the tenant in one
// statement. Sent counts come from the campaign's 'sent' recipients, the five
// feedback counts from the distinct recipient addresses per event kind. A
// campaign with no recipient or event gets zeros. The recompute is idempotent.
//
// ponytail: it reads every recipient and delivery event of the tenant on each
// refresh. Fine for one organisation; the upgrade path is counters kept on the
// campaign row, or a refresh that only reads events newer than the last one.
func (r *YDBAnalytics) Refresh(ctx context.Context, tenantID string) error {
	if err := r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
$sent = SELECT campaign_id, COUNT(*) AS n FROM campaign_recipients
        WHERE tenant_id = $tenant_id AND status = "sent"u GROUP BY campaign_id;
$events = SELECT campaign_id,
                 COUNT(DISTINCT IF(event_kind = "delivery"u, recipient_email)) AS delivered,
                 COUNT(DISTINCT IF(event_kind = "open"u, recipient_email)) AS opened,
                 COUNT(DISTINCT IF(event_kind = "click"u, recipient_email)) AS clicked,
                 COUNT(DISTINCT IF(event_kind = "bounce"u, recipient_email)) AS bounced,
                 COUNT(DISTINCT IF(event_kind = "complaint"u, recipient_email)) AS complained
          FROM delivery_events
          WHERE tenant_id = $tenant_id AND campaign_id IS NOT NULL GROUP BY campaign_id;
UPSERT INTO campaign_analytics
  (tenant_id, campaign_id, sent_count, delivered_count, opened_count, clicked_count,
   bounced_count, complained_count, refreshed_at)
SELECT c.tenant_id AS tenant_id, c.id AS campaign_id,
       Unwrap(CAST(COALESCE(s.n, 0ul) AS Int64)) AS sent_count,
       Unwrap(CAST(COALESCE(e.delivered, 0ul) AS Int64)) AS delivered_count,
       Unwrap(CAST(COALESCE(e.opened, 0ul) AS Int64)) AS opened_count,
       Unwrap(CAST(COALESCE(e.clicked, 0ul) AS Int64)) AS clicked_count,
       Unwrap(CAST(COALESCE(e.bounced, 0ul) AS Int64)) AS bounced_count,
       Unwrap(CAST(COALESCE(e.complained, 0ul) AS Int64)) AS complained_count,
       CurrentUtcTimestamp() AS refreshed_at
FROM campaigns AS c
LEFT JOIN $sent AS s ON s.campaign_id = c.id
LEFT JOIN $events AS e ON e.campaign_id = c.id
WHERE c.tenant_id = $tenant_id AND c.status != "draft"u;`, ydbTenantParam(tenantID)); err != nil {
		return fmt.Errorf("refreshing campaign analytics: %w", err)
	}
	return nil
}

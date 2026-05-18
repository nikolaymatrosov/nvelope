package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/db"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// Analytics is the pgx-backed implementation of domain.AnalyticsRepository.
// Every operation runs inside the RLS-bound tenant transaction, so reads and
// the refresh recompute cannot mix tenants.
type Analytics struct {
	pool *pgxpool.Pool
}

var _ domain.AnalyticsRepository = (*Analytics)(nil)

// NewAnalytics builds an Analytics repository over the given pool.
func NewAnalytics(pool *pgxpool.Pool) *Analytics {
	return &Analytics{pool: pool}
}

// GetCampaign returns one campaign's pre-computed roll-up; ok is false when the
// campaign has no analytics row yet.
func (r *Analytics) GetCampaign(ctx context.Context, tenantID, campaignID string) (
	domain.CampaignAnalytics, bool, error) {

	var out domain.CampaignAnalytics
	var found bool
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		var c domain.Counts
		var refreshedAt time.Time
		err := tx.QueryRow(ctx,
			`SELECT sent_count, delivered_count, opened_count, clicked_count,
			        bounced_count, complained_count, refreshed_at
			 FROM campaign_analytics WHERE campaign_id = $1`,
			campaignID).Scan(&c.Sent, &c.Delivered, &c.Opened, &c.Clicked,
			&c.Bounced, &c.Complained, &refreshedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			if db.IsInvalidInput(err) {
				return nil
			}
			return fmt.Errorf("loading campaign analytics: %w", err)
		}
		out = domain.CampaignAnalytics{CampaignID: campaignID, Counts: c, RefreshedAt: refreshedAt}
		found = true
		return nil
	})
	if err != nil {
		return domain.CampaignAnalytics{}, false, err
	}
	return out, found, nil
}

// GetDashboard returns the tenant totals and the most recently sent campaigns.
func (r *Analytics) GetDashboard(ctx context.Context, tenantID string) (domain.Dashboard, error) {
	var dash domain.Dashboard
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		var t domain.Counts
		if err := tx.QueryRow(ctx,
			`SELECT coalesce(sum(sent_count), 0), coalesce(sum(delivered_count), 0),
			        coalesce(sum(opened_count), 0), coalesce(sum(clicked_count), 0),
			        coalesce(sum(bounced_count), 0), coalesce(sum(complained_count), 0)
			 FROM campaign_analytics`).Scan(&t.Sent, &t.Delivered, &t.Opened,
			&t.Clicked, &t.Bounced, &t.Complained); err != nil {
			return fmt.Errorf("aggregating dashboard totals: %w", err)
		}
		dash.Totals = t

		rows, err := tx.Query(ctx,
			`SELECT ca.campaign_id, c.name, ca.sent_count, ca.delivered_count,
			        ca.opened_count, ca.clicked_count, ca.bounced_count, ca.complained_count
			 FROM campaign_analytics ca
			 JOIN campaigns c ON c.id = ca.campaign_id
			 ORDER BY coalesce(c.started_at, c.created_at) DESC
			 LIMIT 10`)
		if err != nil {
			return fmt.Errorf("listing recent campaigns: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var rc domain.RecentCampaign
			var c domain.Counts
			if err := rows.Scan(&rc.CampaignID, &rc.Name, &c.Sent, &c.Delivered,
				&c.Opened, &c.Clicked, &c.Bounced, &c.Complained); err != nil {
				return fmt.Errorf("scanning recent campaign: %w", err)
			}
			rc.Counts = c
			dash.Recent = append(dash.Recent, rc)
		}
		return rows.Err()
	})
	if err != nil {
		return domain.Dashboard{}, err
	}
	return dash, nil
}

// Refresh recomputes every campaign_analytics row for the tenant. The whole
// recompute is one idempotent upsert inside the tenant transaction — sent
// counts come from campaign_recipients, the five feedback counts from distinct
// recipients per event_kind in delivery_events.
func (r *Analytics) Refresh(ctx context.Context, tenantID string) error {
	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO campaign_analytics
			   (campaign_id, tenant_id, sent_count, delivered_count, opened_count,
			    clicked_count, bounced_count, complained_count, refreshed_at)
			 SELECT c.id, c.tenant_id,
			   (SELECT count(*) FROM campaign_recipients cr
			      WHERE cr.campaign_id = c.id AND cr.status = 'sent'),
			   (SELECT count(DISTINCT de.recipient_email) FROM delivery_events de
			      WHERE de.campaign_id = c.id AND de.event_kind = 'delivery'),
			   (SELECT count(DISTINCT de.recipient_email) FROM delivery_events de
			      WHERE de.campaign_id = c.id AND de.event_kind = 'open'),
			   (SELECT count(DISTINCT de.recipient_email) FROM delivery_events de
			      WHERE de.campaign_id = c.id AND de.event_kind = 'click'),
			   (SELECT count(DISTINCT de.recipient_email) FROM delivery_events de
			      WHERE de.campaign_id = c.id AND de.event_kind = 'bounce'),
			   (SELECT count(DISTINCT de.recipient_email) FROM delivery_events de
			      WHERE de.campaign_id = c.id AND de.event_kind = 'complaint'),
			   now()
			 FROM campaigns c
			 WHERE c.status <> 'draft'
			 ON CONFLICT (campaign_id) DO UPDATE SET
			   sent_count = EXCLUDED.sent_count,
			   delivered_count = EXCLUDED.delivered_count,
			   opened_count = EXCLUDED.opened_count,
			   clicked_count = EXCLUDED.clicked_count,
			   bounced_count = EXCLUDED.bounced_count,
			   complained_count = EXCLUDED.complained_count,
			   refreshed_at = now()`)
		if err != nil {
			return fmt.Errorf("refreshing campaign analytics: %w", err)
		}
		return nil
	})
}

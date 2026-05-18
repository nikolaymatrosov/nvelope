package adapters

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/db"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// Tracking is the pgx-backed implementation of domain.TrackingRepository.
type Tracking struct {
	pool *pgxpool.Pool
}

var _ domain.TrackingRepository = (*Tracking)(nil)

// NewTracking builds a Tracking repository over the given pool.
func NewTracking(pool *pgxpool.Pool) *Tracking {
	return &Tracking{pool: pool}
}

// UpsertLinks ensures one links row per distinct URL and returns the map from
// URL to its links-row id.
func (r *Tracking) UpsertLinks(ctx context.Context, tenantID, campaignID string,
	urls []string) (map[string]string, error) {

	out := make(map[string]string, len(urls))
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		for _, url := range urls {
			var id string
			err := tx.QueryRow(ctx,
				`INSERT INTO links (tenant_id, campaign_id, url) VALUES ($1, $2, $3)
				 ON CONFLICT (campaign_id, url) DO UPDATE SET url = EXCLUDED.url
				 RETURNING id`,
				tenantID, campaignID, url).Scan(&id)
			if err != nil {
				return fmt.Errorf("upserting link: %w", err)
			}
			out[url] = id
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// RecordClick records a click and returns the link's original URL.
func (r *Tracking) RecordClick(ctx context.Context, tenantID, linkID, recipientID string) (string, error) {
	var url string
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		var campaignID string
		err := tx.QueryRow(ctx,
			"SELECT campaign_id, url FROM links WHERE id = $1", linkID).Scan(&campaignID, &url)
		if errors.Is(err, pgx.ErrNoRows) || db.IsInvalidInput(err) {
			return domain.ErrLinkNotFound
		}
		if err != nil {
			return fmt.Errorf("loading link: %w", err)
		}
		_, err = tx.Exec(ctx,
			`INSERT INTO link_clicks (tenant_id, link_id, campaign_id, recipient_id)
			 VALUES ($1, $2, $3, $4)`,
			tenantID, linkID, campaignID, recipientID)
		if err != nil {
			return fmt.Errorf("recording click: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return url, nil
}

// RecordView records an open for one recipient of a campaign.
func (r *Tracking) RecordView(ctx context.Context, tenantID, campaignID, recipientID string) error {
	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO campaign_views (tenant_id, campaign_id, recipient_id)
			 VALUES ($1, $2, $3)`,
			tenantID, campaignID, recipientID)
		if err != nil {
			return fmt.Errorf("recording view: %w", err)
		}
		return nil
	})
}

// ResolveTenantForLink returns the tenant that owns a links row. It runs
// outside any tenant transaction, via a SECURITY DEFINER function that bypasses
// RLS, so the public tracking handler can discover the tenant before binding.
func (r *Tracking) ResolveTenantForLink(ctx context.Context, linkID string) (string, error) {
	var tenantID *string
	err := r.pool.QueryRow(ctx, "SELECT tracking_tenant_for_link($1)", linkID).Scan(&tenantID)
	if db.IsInvalidInput(err) {
		return "", domain.ErrLinkNotFound
	}
	if err != nil {
		return "", fmt.Errorf("resolving tenant for link: %w", err)
	}
	if tenantID == nil {
		return "", domain.ErrLinkNotFound
	}
	return *tenantID, nil
}

// ResolveTenantForCampaign returns the tenant that owns a campaign.
func (r *Tracking) ResolveTenantForCampaign(ctx context.Context, campaignID string) (string, error) {
	var tenantID *string
	err := r.pool.QueryRow(ctx, "SELECT tracking_tenant_for_campaign($1)", campaignID).Scan(&tenantID)
	if db.IsInvalidInput(err) {
		return "", domain.ErrCampaignNotFound
	}
	if err != nil {
		return "", fmt.Errorf("resolving tenant for campaign: %w", err)
	}
	if tenantID == nil {
		return "", domain.ErrCampaignNotFound
	}
	return *tenantID, nil
}

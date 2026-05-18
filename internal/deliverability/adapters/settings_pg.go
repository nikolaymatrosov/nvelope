package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// Settings is the pgx-backed implementation of domain.SettingsRepository.
type Settings struct {
	pool *pgxpool.Pool
}

var _ domain.SettingsRepository = (*Settings)(nil)

// NewSettings builds a Settings repository over the given pool.
func NewSettings(pool *pgxpool.Pool) *Settings {
	return &Settings{pool: pool}
}

// Get returns the tenant's bounce settings, or DefaultBounceSettings when no
// row exists.
func (r *Settings) Get(ctx context.Context, tenantID string) (*domain.BounceSettings, error) {
	var suppressHard, suppressComplaint bool
	var updatedAt time.Time
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT suppress_on_hard_bounce, suppress_on_complaint, updated_at
			 FROM bounce_settings WHERE tenant_id = $1`,
			tenantID).Scan(&suppressHard, &suppressComplaint, &updatedAt)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DefaultBounceSettings(tenantID), nil
	}
	if err != nil {
		return nil, fmt.Errorf("loading bounce settings: %w", err)
	}
	return domain.HydrateBounceSettings(tenantID, suppressHard, suppressComplaint, updatedAt), nil
}

// Put upserts the tenant's bounce settings.
func (r *Settings) Put(ctx context.Context, tenantID string, s *domain.BounceSettings) error {
	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO bounce_settings
			   (tenant_id, suppress_on_hard_bounce, suppress_on_complaint, updated_at)
			 VALUES ($1, $2, $3, now())
			 ON CONFLICT (tenant_id) DO UPDATE
			 SET suppress_on_hard_bounce = EXCLUDED.suppress_on_hard_bounce,
			     suppress_on_complaint = EXCLUDED.suppress_on_complaint,
			     updated_at = now()`,
			tenantID, s.SuppressOnHardBounce(), s.SuppressOnComplaint())
		if err != nil {
			return fmt.Errorf("upserting bounce settings: %w", err)
		}
		return nil
	})
}

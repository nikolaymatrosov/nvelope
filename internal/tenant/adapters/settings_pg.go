package adapters

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

// Settings is the pgx-backed implementation of domain.SettingsRepository. Both
// operations run inside a tenant-bound (app.tenant_id) transaction — this
// adapter owns the Row-Level-Security binding.
type Settings struct {
	pool *pgxpool.Pool
}

var _ domain.SettingsRepository = (*Settings)(nil)

// NewSettings builds a Settings repository over the given pool.
func NewSettings(pool *pgxpool.Pool) *Settings {
	return &Settings{pool: pool}
}

// Get returns the bound tenant's settings, or domain.ErrTenantNotFound. RLS
// guarantees only the bound tenant's row is visible, so the query needs no
// tenant_id filter.
func (r *Settings) Get(ctx context.Context, tenantID string) (*domain.TenantSettings, error) {
	var settings *domain.TenantSettings
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		var displayName, timezone, rawHTML string
		err := tx.QueryRow(ctx,
			"SELECT display_name, timezone, raw_html_sanitization FROM tenant_settings").
			Scan(&displayName, &timezone, &rawHTML)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrTenantNotFound
		}
		if err != nil {
			return fmt.Errorf("loading tenant settings: %w", err)
		}
		settings = domain.HydrateTenantSettings(tenantID, displayName, timezone, domain.RawHTMLSanitization(rawHTML))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return settings, nil
}

// Update loads the bound tenant's settings, runs fn, and persists the result.
// The whole load-mutate-save runs inside one tenant-bound transaction; the RLS
// WITH CHECK clause makes it impossible to write another tenant's row.
func (r *Settings) Update(ctx context.Context, tenantID string,
	fn func(*domain.TenantSettings) (*domain.TenantSettings, error)) error {

	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		var displayName, timezone, rawHTML string
		err := tx.QueryRow(ctx,
			"SELECT display_name, timezone, raw_html_sanitization FROM tenant_settings FOR UPDATE").
			Scan(&displayName, &timezone, &rawHTML)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrTenantNotFound
		}
		if err != nil {
			return fmt.Errorf("loading tenant settings: %w", err)
		}

		loaded := domain.HydrateTenantSettings(tenantID, displayName, timezone, domain.RawHTMLSanitization(rawHTML))
		updated, err := fn(loaded)
		if err != nil {
			return err
		}

		if _, err := tx.Exec(ctx,
			`UPDATE tenant_settings
			    SET display_name = $1, timezone = $2, raw_html_sanitization = $3, updated_at = now()`,
			updated.DisplayName(), updated.Timezone(), string(updated.RawHTMLSanitization())); err != nil {
			return fmt.Errorf("updating tenant settings: %w", err)
		}
		return nil
	})
}

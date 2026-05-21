package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

// Branding is the pgx-backed implementation of domain.BrandingRepository.
type Branding struct {
	pool *pgxpool.Pool
}

var _ domain.BrandingRepository = (*Branding)(nil)

// NewBranding builds a Branding repository over the pool.
func NewBranding(pool *pgxpool.Pool) *Branding {
	return &Branding{pool: pool}
}

// Get returns the tenant's branding, or an empty (platform-default) value when
// no row exists.
func (r *Branding) Get(ctx context.Context, tenantID string) (*domain.TenantBranding, error) {
	var out *domain.TenantBranding
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		var logoURL, primaryColor, customCSS string
		var updatedAt time.Time
		err := tx.QueryRow(ctx,
			`SELECT logo_url, primary_color, custom_css, updated_at
			 FROM tenant_branding WHERE tenant_id = $1`, tenantID).
			Scan(&logoURL, &primaryColor, &customCSS, &updatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			out = domain.NewTenantBranding(tenantID)
			return nil
		}
		if err != nil {
			return fmt.Errorf("loading tenant branding: %w", err)
		}
		out = domain.HydrateTenantBranding(tenantID, logoURL, primaryColor, customCSS, updatedAt)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Save upserts the tenant's branding.
func (r *Branding) Save(ctx context.Context, b *domain.TenantBranding) error {
	return tenantdb.WithTenant(ctx, r.pool, b.TenantID(), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO tenant_branding (tenant_id, logo_url, primary_color, custom_css)
			 VALUES (@tenant_id, @logo_url, @primary_color, @custom_css)
			 ON CONFLICT (tenant_id) DO UPDATE
			   SET logo_url = EXCLUDED.logo_url,
			       primary_color = EXCLUDED.primary_color,
			       custom_css = EXCLUDED.custom_css,
			       updated_at = now()`,
			pgx.NamedArgs{
				"tenant_id":     b.TenantID(),
				"logo_url":      b.LogoURL(),
				"primary_color": b.PrimaryColor(),
				"custom_css":    b.CustomCSS(),
			})
		if err != nil {
			return fmt.Errorf("saving tenant branding: %w", err)
		}
		return nil
	})
}

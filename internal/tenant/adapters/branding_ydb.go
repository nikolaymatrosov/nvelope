package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

// YDBBranding is the YDB-backed implementation of domain.BrandingRepository,
// for the serverless runtime. YDB has no row-level security, so every
// statement filters by the tenant_id primary key explicitly.
type YDBBranding struct {
	driver *ydb.Driver
}

var _ domain.BrandingRepository = (*YDBBranding)(nil)

// NewYDBBranding builds a YDBBranding repository over the driver.
func NewYDBBranding(d *ydb.Driver) *YDBBranding {
	return &YDBBranding{driver: d}
}

// Get returns the tenant's branding, or an empty (platform-default) value when
// no row exists.
func (r *YDBBranding) Get(ctx context.Context, tenantID string) (*domain.TenantBranding, error) {
	row, err := r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
SELECT logo_url, primary_color, custom_css, updated_at
FROM tenant_branding
WHERE tenant_id = $tenant_id;`, ydbTenantParam(tenantID))
	if errors.Is(err, query.ErrNoRows) {
		return domain.NewTenantBranding(tenantID), nil
	}
	if err != nil {
		return nil, fmt.Errorf("loading tenant branding: %w", err)
	}
	var logoURL, primaryColor, customCSS string
	var updatedAt time.Time
	if err := row.Scan(&logoURL, &primaryColor, &customCSS, &updatedAt); err != nil {
		return nil, fmt.Errorf("scanning tenant branding: %w", err)
	}
	return domain.HydrateTenantBranding(tenantID, logoURL, primaryColor, customCSS, updatedAt), nil
}

// Save upserts the tenant's branding row, replacing every field.
func (r *YDBBranding) Save(ctx context.Context, b *domain.TenantBranding) error {
	if err := r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $logo_url AS Utf8;
DECLARE $primary_color AS Utf8;
DECLARE $custom_css AS Utf8;
UPSERT INTO tenant_branding (tenant_id, logo_url, primary_color, custom_css, updated_at)
VALUES ($tenant_id, $logo_url, $primary_color, $custom_css, CurrentUtcTimestamp());`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(b.TenantID()).
			Param("$logo_url").Text(b.LogoURL()).
			Param("$primary_color").Text(b.PrimaryColor()).
			Param("$custom_css").Text(b.CustomCSS()).
			Build())); err != nil {
		return fmt.Errorf("saving tenant branding: %w", err)
	}
	return nil
}

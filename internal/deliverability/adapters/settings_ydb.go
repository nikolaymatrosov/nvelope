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

// YDBSettings is the YDB-backed implementation of domain.SettingsRepository,
// for the serverless runtime.
type YDBSettings struct {
	driver *ydb.Driver
}

var _ domain.SettingsRepository = (*YDBSettings)(nil)

// NewYDBSettings builds a YDBSettings repository over the driver.
func NewYDBSettings(d *ydb.Driver) *YDBSettings {
	return &YDBSettings{driver: d}
}

// Get returns the tenant's bounce settings, or DefaultBounceSettings when no
// row exists.
func (r *YDBSettings) Get(ctx context.Context, tenantID string) (*domain.BounceSettings, error) {
	row, err := r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
SELECT suppress_on_hard_bounce, suppress_on_complaint, updated_at
FROM bounce_settings WHERE tenant_id = $tenant_id;`, ydbTenantParam(tenantID))
	if errors.Is(err, query.ErrNoRows) {
		return domain.DefaultBounceSettings(tenantID), nil
	}
	if err != nil {
		return nil, fmt.Errorf("loading bounce settings: %w", err)
	}
	var suppressHard, suppressComplaint bool
	var updatedAt time.Time
	if err := row.Scan(&suppressHard, &suppressComplaint, &updatedAt); err != nil {
		return nil, fmt.Errorf("scanning bounce settings: %w", err)
	}
	return domain.HydrateBounceSettings(tenantID, suppressHard, suppressComplaint, updatedAt), nil
}

// Put upserts the tenant's bounce settings.
func (r *YDBSettings) Put(ctx context.Context, tenantID string, s *domain.BounceSettings) error {
	if err := r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $suppress_on_hard_bounce AS Bool;
DECLARE $suppress_on_complaint AS Bool;
UPSERT INTO bounce_settings (tenant_id, suppress_on_hard_bounce, suppress_on_complaint, updated_at)
VALUES ($tenant_id, $suppress_on_hard_bounce, $suppress_on_complaint, CurrentUtcTimestamp());`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$suppress_on_hard_bounce").Bool(s.SuppressOnHardBounce()).
			Param("$suppress_on_complaint").Bool(s.SuppressOnComplaint()).
			Build())); err != nil {
		return fmt.Errorf("upserting bounce settings: %w", err)
	}
	return nil
}

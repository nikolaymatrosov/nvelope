package adapters

import (
	"context"
	"errors"
	"fmt"

	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

// YDBSettings is the YDB-backed implementation of domain.SettingsRepository,
// for the serverless runtime. YDB has no row-level security, so every
// statement filters by the tenant_id primary key explicitly.
type YDBSettings struct {
	driver *ydb.Driver
}

var _ domain.SettingsRepository = (*YDBSettings)(nil)

// NewYDBSettings builds a YDBSettings repository over the driver.
func NewYDBSettings(d *ydb.Driver) *YDBSettings {
	return &YDBSettings{driver: d}
}

const ydbSelectSettings = `DECLARE $tenant_id AS Utf8;
SELECT display_name, timezone, raw_html_sanitization
FROM tenant_settings
WHERE tenant_id = $tenant_id;`

// Get returns the tenant's settings, or domain.ErrTenantNotFound.
func (r *YDBSettings) Get(ctx context.Context, tenantID string) (*domain.TenantSettings, error) {
	row, err := r.driver.Query().QueryRow(ctx, ydbSelectSettings, ydbTenantParam(tenantID))
	return ydbSettingsFromRow(tenantID, row, err)
}

// Update loads the tenant's settings, runs fn, and persists the result in one
// serializable transaction. The SDK retries the transaction on conflict, so fn
// may run more than once and must be retry-safe.
func (r *YDBSettings) Update(ctx context.Context, tenantID string,
	fn func(*domain.TenantSettings) (*domain.TenantSettings, error)) error {

	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		row, err := tx.QueryRow(ctx, ydbSelectSettings, ydbTenantParam(tenantID))
		loaded, err := ydbSettingsFromRow(tenantID, row, err)
		if err != nil {
			return err
		}
		updated, err := fn(loaded)
		if err != nil {
			return err
		}
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $display_name AS Utf8;
DECLARE $timezone AS Utf8;
DECLARE $raw_html_sanitization AS Utf8;
UPDATE tenant_settings
SET display_name = $display_name, timezone = $timezone,
    raw_html_sanitization = $raw_html_sanitization, updated_at = CurrentUtcTimestamp()
WHERE tenant_id = $tenant_id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$display_name").Text(updated.DisplayName()).
				Param("$timezone").Text(updated.Timezone()).
				Param("$raw_html_sanitization").Text(string(updated.RawHTMLSanitization())).
				Build())); err != nil {
			return fmt.Errorf("updating tenant settings: %w", err)
		}
		return nil
	})
}

// ydbSettingsFromRow maps a single-row read in ydbSelectSettings order to the
// settings, translating "no row" into domain.ErrTenantNotFound.
func ydbSettingsFromRow(tenantID string, row query.Row, err error) (*domain.TenantSettings, error) {
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrTenantNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading tenant settings: %w", err)
	}
	var displayName, timezone, rawHTML string
	if err := row.Scan(&displayName, &timezone, &rawHTML); err != nil {
		return nil, fmt.Errorf("scanning tenant settings: %w", err)
	}
	return domain.HydrateTenantSettings(tenantID, displayName, timezone,
		domain.RawHTMLSanitization(rawHTML)), nil
}

package tenant

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/nvelope/nvelope/internal/db"
)

// Settings holds a tenant's settings. tenant_settings is the first
// tenant-plane table — every read and write below is RLS-confined to the
// tenant bound by WithTenant.
type Settings struct {
	DisplayName string `json:"display_name"`
	Timezone    string `json:"timezone"`
}

// createSettings inserts a tenant's initial settings row. It MUST run inside a
// transaction already bound to tenantID (app.tenant_id set), so the RLS
// WITH CHECK clause accepts the insert.
func createSettings(ctx context.Context, q db.Querier, tenantID, displayName string) error {
	if _, err := q.Exec(ctx,
		`INSERT INTO tenant_settings (tenant_id, display_name) VALUES ($1, $2)`,
		tenantID, displayName); err != nil {
		return fmt.Errorf("inserting tenant settings: %w", err)
	}
	return nil
}

// GetSettings returns the bound tenant's settings. It MUST run inside a
// WithTenant transaction; RLS guarantees only the bound tenant's row is
// visible, so the query needs no tenant_id filter.
func GetSettings(ctx context.Context, q db.Querier) (Settings, error) {
	var s Settings
	err := q.QueryRow(ctx,
		"SELECT display_name, timezone FROM tenant_settings").
		Scan(&s.DisplayName, &s.Timezone)
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{}, ErrTenantNotFound
	}
	if err != nil {
		return Settings{}, fmt.Errorf("loading tenant settings: %w", err)
	}
	return s, nil
}

// UpdateSettings updates the bound tenant's settings. It MUST run inside a
// WithTenant transaction; the RLS WITH CHECK clause makes it impossible to
// write another tenant's row.
func UpdateSettings(ctx context.Context, q db.Querier, s Settings) error {
	displayName := strings.TrimSpace(s.DisplayName)
	if displayName == "" {
		return ValidationError{"display name is required"}
	}
	timezone := strings.TrimSpace(s.Timezone)
	if timezone == "" {
		return ValidationError{"timezone is required"}
	}
	tag, err := q.Exec(ctx,
		`UPDATE tenant_settings SET display_name = $1, timezone = $2, updated_at = now()`,
		displayName, timezone)
	if err != nil {
		return fmt.Errorf("updating tenant settings: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrTenantNotFound
	}
	return nil
}

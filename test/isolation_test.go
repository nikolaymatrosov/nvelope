// The cross-tenant isolation suite. It connects as the restricted nvelope_app
// role and proves that operations bound to one tenant cannot read, modify, or
// delete another tenant's rows — even when the application-level tenant filter
// is deliberately omitted (spec FR-016, FR-017, FR-019; Constitution I).
package test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nvelope/nvelope/internal/dbtest"
	"github.com/nvelope/nvelope/internal/tenant"
)

func TestCrossTenantIsolation(t *testing.T) {
	pool := dbtest.AppPool(t)
	ctx := context.Background()

	// Two tenants, each created with its own tenant_settings row.
	tenantA := seedTenant(t, pool, "Tenant A")
	tenantB := seedTenant(t, pool, "Tenant B")

	// 1. SELECT without a tenant filter, bound to A → only A's row is visible.
	require.NoError(t, tenant.WithTenant(ctx, pool, tenantA, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "SELECT tenant_id FROM tenant_settings")
		require.NoError(t, err)
		defer rows.Close()

		var visible []string
		for rows.Next() {
			var id string
			require.NoError(t, rows.Scan(&id))
			visible = append(visible, id)
		}
		require.NoError(t, rows.Err())
		require.Equal(t, []string{tenantA}, visible,
			"bound to A, an unfiltered SELECT returns only A's row")
		return nil
	}))

	// 2. UPDATE without a tenant filter, bound to A → B's row is untouched.
	require.NoError(t, tenant.WithTenant(ctx, pool, tenantA, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "UPDATE tenant_settings SET display_name = 'hijacked'")
		require.NoError(t, err)
		require.EqualValues(t, 1, tag.RowsAffected(),
			"the unfiltered UPDATE reached only the bound tenant's row")
		return nil
	}))
	require.Equal(t, "Tenant B", displayName(t, pool, tenantB),
		"tenant B's display name is unchanged by tenant A's unfiltered UPDATE")

	// 3. DELETE without a tenant filter, bound to A → B's row survives.
	require.NoError(t, tenant.WithTenant(ctx, pool, tenantA, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "DELETE FROM tenant_settings")
		require.NoError(t, err)
		require.EqualValues(t, 1, tag.RowsAffected(),
			"the unfiltered DELETE removed only the bound tenant's row")
		return nil
	}))
	require.Equal(t, "Tenant B", displayName(t, pool, tenantB),
		"tenant B's row survives tenant A's unfiltered DELETE")

	// 4. INSERT targeting tenant B while bound to A → rejected by WITH CHECK.
	err := tenant.WithTenant(ctx, pool, tenantA, func(ctx context.Context, tx pgx.Tx) error {
		_, e := tx.Exec(ctx,
			"INSERT INTO tenant_settings (tenant_id, display_name) VALUES ($1, $2)",
			tenantB, "smuggled")
		return e
	})
	require.Error(t, err,
		"an INSERT writing another tenant's id must be rejected by the RLS WITH CHECK")
}

func TestTenantPlaneFailsClosedWithoutBinding(t *testing.T) {
	pool := dbtest.AppPool(t)
	seedTenant(t, pool, "Some Tenant")

	// A query outside a tenant-bound transaction has no app.tenant_id set:
	// RLS exposes zero rows (deny by default).
	var count int
	require.NoError(t, pool.QueryRow(context.Background(),
		"SELECT count(*) FROM tenant_settings").Scan(&count))
	require.Equal(t, 0, count, "unbound access to a tenant-plane table sees nothing")
}

// seedTenant creates a platform user, a tenant owned by them, and returns the
// tenant id. CreateTenant also writes the tenant's tenant_settings row.
func seedTenant(t *testing.T, pool *pgxpool.Pool, name string) string {
	t.Helper()
	ctx := context.Background()

	var ownerID string
	require.NoError(t, pool.QueryRow(ctx,
		"INSERT INTO platform_users (email, password_hash, name) VALUES ($1, $2, $3) RETURNING id",
		dbtest.RandString()+"@example.com", "x", "Owner").Scan(&ownerID))

	tn, err := tenant.CreateTenant(ctx, pool, ownerID, name, "iso-"+dbtest.RandString())
	require.NoError(t, err)
	return tn.ID
}

// displayName reads a tenant's settings display_name inside its own bound
// transaction.
func displayName(t *testing.T, pool *pgxpool.Pool, tenantID string) string {
	t.Helper()
	var name string
	require.NoError(t, tenant.WithTenant(context.Background(), pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, "SELECT display_name FROM tenant_settings").Scan(&name)
		}))
	return name
}

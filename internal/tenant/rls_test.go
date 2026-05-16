package tenant

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/nvelope/nvelope/internal/dbtest"
)

func TestWithTenantBindsTransaction(t *testing.T) {
	pool := dbtest.AppPool(t)
	ctx := context.Background()

	tenantA := insertTestTenant(t, pool)
	tenantB := insertTestTenant(t, pool)

	// Insert a settings row for tenant A inside its bound transaction.
	require.NoError(t, WithTenant(ctx, pool, tenantA, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			"INSERT INTO tenant_settings (tenant_id, display_name) VALUES ($1, $2)",
			tenantA, "Tenant A")
		return err
	}))

	count := func(boundTenant string) int {
		var n int
		require.NoError(t, WithTenant(ctx, pool, boundTenant, func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, "SELECT count(*) FROM tenant_settings").Scan(&n)
		}))
		return n
	}

	require.Equal(t, 1, count(tenantA), "bound to A, A's row is visible")
	require.Equal(t, 0, count(tenantB), "bound to B, A's row is invisible — RLS confines it")
}

func TestWithTenantRollsBackOnError(t *testing.T) {
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := insertTestTenant(t, pool)

	sentinel := errors.New("boom")
	err := WithTenant(ctx, pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			"INSERT INTO tenant_settings (tenant_id, display_name) VALUES ($1, $2)",
			tenantID, "Doomed"); err != nil {
			return err
		}
		return sentinel
	})
	require.ErrorIs(t, err, sentinel)

	var count int
	require.NoError(t, WithTenant(ctx, pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT count(*) FROM tenant_settings").Scan(&count)
	}))
	require.Equal(t, 0, count, "the failed transaction's insert was rolled back")
}

func TestTenantPlaneDeniesUnboundAccess(t *testing.T) {
	pool := dbtest.AppPool(t)
	ctx := context.Background()

	// A query against a tenant-plane table outside WithTenant has no
	// app.tenant_id set: RLS exposes zero rows (fail closed).
	var count int
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM tenant_settings").Scan(&count))
	require.Equal(t, 0, count)
}

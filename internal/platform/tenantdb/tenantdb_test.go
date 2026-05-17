package tenantdb_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// seedTenant inserts a tenant and returns its id, so the test has a real
// tenant-plane row space to read and write.
func seedTenant(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	ctx := context.Background()
	var ownerID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO platform_users (email, password_hash, name)
		 VALUES ($1, 'hash', 'Owner') RETURNING id`,
		dbtest.RandString()+"@example.com").Scan(&ownerID))
	var tenantID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO tenants (name, slug, status) VALUES ('T', $1, 'active') RETURNING id`,
		"tdb-"+dbtest.RandString()).Scan(&tenantID))
	return tenantID
}

func TestWithTenantBindsTenantID(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()

	tenantID := seedTenant(t, pool)

	// Inside WithTenant the GUC is bound, so the WITH CHECK clause accepts an
	// insert of the bound tenant's settings row.
	require.NoError(t, tenantdb.WithTenant(ctx, pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		var bound string
		if err := tx.QueryRow(ctx, "SELECT current_setting('app.tenant_id', true)").Scan(&bound); err != nil {
			return err
		}
		require.Equal(t, tenantID, bound, "the transaction binds app.tenant_id")
		_, err := tx.Exec(ctx,
			"INSERT INTO tenant_settings (tenant_id, display_name) VALUES ($1, 'W')", tenantID)
		return err
	}))

	require.NoError(t, tenantdb.WithTenant(ctx, pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM tenant_settings").Scan(&n); err != nil {
			return err
		}
		require.Equal(t, 1, n, "the bound tenant sees its own row")
		return nil
	}))
}

func TestWithTenantFailsClosedWhenUnset(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()

	tenantID := seedTenant(t, pool)
	require.NoError(t, tenantdb.WithTenant(ctx, pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			"INSERT INTO tenant_settings (tenant_id, display_name) VALUES ($1, 'W')", tenantID)
		return err
	}))

	// A read outside any tenant-bound transaction has no app.tenant_id: RLS
	// exposes zero rows — isolation fails closed.
	var n int
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM tenant_settings").Scan(&n))
	require.Equal(t, 0, n, "unbound access to a tenant-plane table sees nothing")
}

func TestWithTenantRollsBackOnError(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()

	tenantID := seedTenant(t, pool)
	sentinel := errors.New("closure failed")
	err := tenantdb.WithTenant(ctx, pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			"INSERT INTO tenant_settings (tenant_id, display_name) VALUES ($1, 'W')", tenantID); err != nil {
			return err
		}
		return sentinel
	})
	require.ErrorIs(t, err, sentinel)

	require.NoError(t, tenantdb.WithTenant(ctx, pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM tenant_settings").Scan(&n); err != nil {
			return err
		}
		require.Equal(t, 0, n, "a closure error rolls the transaction back")
		return nil
	}))
}

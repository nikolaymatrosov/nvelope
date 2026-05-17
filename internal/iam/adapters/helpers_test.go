package adapters_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/iam/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// seedList inserts a list row inside a tenant-bound transaction and returns
// its id — needed to satisfy the user_list_roles foreign key.
func seedList(t *testing.T, pool *pgxpool.Pool, tenantID string) string {
	t.Helper()
	var id string
	require.NoError(t, tenantdb.WithTenant(context.Background(), pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx,
				"INSERT INTO lists (tenant_id, name) VALUES ($1, $2) RETURNING id",
				tenantID, "L-"+dbtest.RandString()).Scan(&id)
		}))
	return id
}

// seedTenant inserts a tenant row directly and returns its id.
func seedTenant(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO tenants (name, slug, status) VALUES ('Workspace', $1, 'active') RETURNING id`,
		"iam-"+dbtest.RandString()).Scan(&id))
	return id
}

// seedPlatformUser inserts a control-plane user row directly and returns its id.
func seedPlatformUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO platform_users (email, password_hash, name)
		 VALUES ($1, 'hash', 'User') RETURNING id`,
		dbtest.RandString()+"@example.com").Scan(&id))
	return id
}

// seedTenantUser inserts a tenant-plane user and returns its id.
func seedTenantUser(t *testing.T, pool *pgxpool.Pool, tenantID string) string {
	t.Helper()
	platformID := seedPlatformUser(t, pool)
	u, err := domain.NewTenantUser(tenantID, platformID, dbtest.RandString()+"@example.com", "Member")
	require.NoError(t, err)
	id, err := adapters.NewUsers(pool).Add(context.Background(), tenantID, u)
	require.NoError(t, err)
	return id
}

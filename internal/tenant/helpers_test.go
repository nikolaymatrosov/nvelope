package tenant

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nvelope/nvelope/internal/dbtest"
)

// insertTestTenant inserts a tenant directly (control-plane, no RLS) with a
// random slug and returns its id. It is a story-independent fixture: it does
// not rely on the CreateTenant flow under test.
func insertTestTenant(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(),
		"INSERT INTO tenants (slug, name) VALUES ($1, $2) RETURNING id",
		"t-"+dbtest.RandString(), "Test Tenant").Scan(&id))
	return id
}

// insertTestUser inserts a platform user directly and returns its id.
func insertTestUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(),
		"INSERT INTO platform_users (email, password_hash, name) VALUES ($1, $2, $3) RETURNING id",
		dbtest.RandString()+"@example.com", "x", "Test User").Scan(&id))
	return id
}

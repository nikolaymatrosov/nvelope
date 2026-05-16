package adapters_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/tenant/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

// insertUser inserts a platform user directly and returns its id. The tenant
// adapters need real users to satisfy the membership and invitation foreign
// keys; the auth context owns user creation, so the test inserts one directly.
func insertUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO platform_users (email, password_hash, name)
		 VALUES ($1, 'hash', 'Test User') RETURNING id`,
		dbtest.RandString()+"@example.com").Scan(&id)
	require.NoError(t, err)
	return id
}

// createWorkspace creates a tenant with a fresh slug owned by ownerID and
// returns the persisted tenant.
func createWorkspace(t *testing.T, repo *adapters.Tenants, ownerID string) *domain.Tenant {
	t.Helper()
	tn, err := domain.NewTenant("Workspace", "ws-"+dbtest.RandString())
	require.NoError(t, err)
	created, err := repo.CreateWorkspace(context.Background(), tn, ownerID)
	require.NoError(t, err)
	return created
}

package adapters_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

// seedTenant inserts a tenant row directly and returns its id. The sending
// adapters need a real tenant to satisfy the tenant_id foreign key and the RLS
// policy.
func seedTenant(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO tenants (name, slug, status) VALUES ('Workspace', $1, 'active') RETURNING id`,
		"snd-"+dbtest.RandString()).Scan(&id))
	return id
}

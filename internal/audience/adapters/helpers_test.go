package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

// timeZeroAdapters is a fixed timestamp for hydrating entities in tests.
func timeZeroAdapters() time.Time { return time.Unix(0, 0).UTC() }

// seedTenant inserts a tenant row directly and returns its id. The audience
// adapters need a real tenant to satisfy the tenant_id foreign key and the RLS
// policy. The row is inserted with raw SQL rather than through the tenant
// context's adapter so the audience context's tests import no other context.
func seedTenant(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO tenants (name, slug, status) VALUES ('Workspace', $1, 'active') RETURNING id`,
		"aud-"+dbtest.RandString()).Scan(&id))
	return id
}

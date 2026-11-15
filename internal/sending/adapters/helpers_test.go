package adapters_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/sending/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/sending/domain"
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

// backend is one storage implementation of the sending repository under
// contract test. Every contract test body runs once per backend, so the
// Postgres and YDB adapters are held to the same behaviour.
type backend struct {
	name string
	// repo opens the backend's repository.
	repo func(t *testing.T) domain.SendingDomainRepository
	// newTenant returns a fresh tenant id valid for the backend: a real tenants
	// row for Postgres (foreign key + RLS), a bare UUID for YDB (no foreign
	// keys; isolation is the explicit tenant_id predicate).
	newTenant func(t *testing.T) string
}

var backends = []backend{
	{
		name: "pg",
		repo: func(t *testing.T) domain.SendingDomainRepository {
			return adapters.NewSendingDomains(dbtest.AppPool(t))
		},
		newTenant: func(t *testing.T) string { return seedTenant(t, dbtest.AppPool(t)) },
	},
	{
		name: "ydb",
		repo: func(t *testing.T) domain.SendingDomainRepository {
			return adapters.NewYDBSendingDomains(dbtest.YDBMigrated(t))
		},
		newTenant: func(*testing.T) string { return uuid.NewString() },
	},
}

// eachBackend runs body once per backend, as parallel subtests named after it.
func eachBackend(t *testing.T, body func(t *testing.T, b backend)) {
	t.Helper()
	for _, b := range backends {
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()
			body(t, b)
		})
	}
}

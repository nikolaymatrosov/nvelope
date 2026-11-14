package adapters_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/iam/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// backend is one storage implementation of the iam repositories under
// contract test. Every contract test body runs once per backend, so the
// Postgres and YDB adapters are held to the same behaviour.
type backend struct {
	name     string
	users    func(t *testing.T) domain.UserRepository
	sessions func(t *testing.T) domain.SessionRepository
	audit    func(t *testing.T) domain.AuditRepository
	// newTenant and newPlatformUser return ids of rows other contexts own. On
	// Postgres the iam tables carry foreign keys to them, so the pg backend
	// inserts real rows. YDB has no foreign keys and no iam query reads those
	// tables, so a bare UUID stands in.
	newTenant       func(t *testing.T) string
	newPlatformUser func(t *testing.T) string
}

var backends = []backend{
	{
		name: "pg",
		users: func(t *testing.T) domain.UserRepository {
			return adapters.NewUsers(dbtest.AppPool(t))
		},
		sessions: func(t *testing.T) domain.SessionRepository {
			return adapters.NewSessions(dbtest.AppPool(t))
		},
		audit: func(t *testing.T) domain.AuditRepository {
			return adapters.NewAudit(dbtest.AppPool(t))
		},
		newTenant:       func(t *testing.T) string { return seedTenant(t, dbtest.AppPool(t)) },
		newPlatformUser: func(t *testing.T) string { return seedPlatformUser(t, dbtest.AppPool(t)) },
	},
	{
		name: "ydb",
		users: func(t *testing.T) domain.UserRepository {
			return adapters.NewYDBUsers(dbtest.YDBMigrated(t))
		},
		sessions: func(t *testing.T) domain.SessionRepository {
			return adapters.NewYDBSessions(dbtest.YDBMigrated(t))
		},
		audit: func(t *testing.T) domain.AuditRepository {
			return adapters.NewYDBAudit(dbtest.YDBMigrated(t))
		},
		newTenant:       func(*testing.T) string { return uuid.NewString() },
		newPlatformUser: func(*testing.T) string { return uuid.NewString() },
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

// newTenantUser adds a tenant-plane user with a fresh address through the
// backend's user repository and returns its id.
func newTenantUser(t *testing.T, b backend, tenantID string) string {
	t.Helper()
	u, err := domain.NewTenantUser(tenantID, b.newPlatformUser(t), dbtest.RandString()+"@example.com", "Member")
	require.NoError(t, err)
	id, err := b.users(t).Add(context.Background(), tenantID, u)
	require.NoError(t, err)
	return id
}

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

package adapters_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/tenant/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

// testUserName is the name of every platform user newUser seeds.
const testUserName = "Test User"

// backend is one storage implementation of the tenant repositories under
// contract test. Every contract test body runs once per backend, so the
// Postgres and YDB adapters are held to the same behaviour.
type backend struct {
	name        string
	tenants     func(t *testing.T) domain.TenantRepository
	invitations func(t *testing.T) domain.InvitationRepository
	settings    func(t *testing.T) domain.SettingsRepository
	branding    func(t *testing.T) domain.BrandingRepository
	// insertUser inserts a platform user with the given email directly and
	// returns its id. On Postgres, memberships and invitations carry foreign
	// keys to platform_users, and ListMembers joins it on both backends. The
	// auth context owns user creation and its adapters are off-limits here
	// (make lint-arch), so the row is written by hand.
	insertUser func(t *testing.T, email string) string
}

var backends = []backend{
	{
		name: "pg",
		tenants: func(t *testing.T) domain.TenantRepository {
			return adapters.NewTenants(dbtest.AppPool(t))
		},
		invitations: func(t *testing.T) domain.InvitationRepository {
			return adapters.NewInvitations(dbtest.AppPool(t))
		},
		settings: func(t *testing.T) domain.SettingsRepository {
			return adapters.NewSettings(dbtest.AppPool(t))
		},
		branding: func(t *testing.T) domain.BrandingRepository {
			return adapters.NewBranding(dbtest.AppPool(t))
		},
		insertUser: func(t *testing.T, email string) string {
			var id string
			err := dbtest.AppPool(t).QueryRow(context.Background(),
				`INSERT INTO platform_users (email, password_hash, name)
				 VALUES ($1, 'hash', $2) RETURNING id`,
				email, testUserName).Scan(&id)
			require.NoError(t, err)
			return id
		},
	},
	{
		name: "ydb",
		tenants: func(t *testing.T) domain.TenantRepository {
			return adapters.NewYDBTenants(dbtest.YDBMigrated(t))
		},
		invitations: func(t *testing.T) domain.InvitationRepository {
			return adapters.NewYDBInvitations(dbtest.YDBMigrated(t))
		},
		settings: func(t *testing.T) domain.SettingsRepository {
			return adapters.NewYDBSettings(dbtest.YDBMigrated(t))
		},
		branding: func(t *testing.T) domain.BrandingRepository {
			return adapters.NewYDBBranding(dbtest.YDBMigrated(t))
		},
		insertUser: func(t *testing.T, email string) string {
			id := uuid.NewString()
			err := dbtest.YDBMigrated(t).Query().Exec(context.Background(), `DECLARE $id AS Utf8;
DECLARE $email AS Utf8;
DECLARE $email_key AS Utf8;
DECLARE $name AS Utf8;
INSERT INTO platform_users (id, email, email_key, password_hash, name, created_at, updated_at)
VALUES ($id, $email, $email_key, 'hash', $name, CurrentUtcTimestamp(), CurrentUtcTimestamp());`,
				query.WithParameters(ydb.ParamsBuilder().
					Param("$id").Text(id).
					Param("$email").Text(email).
					Param("$email_key").Text(strings.ToLower(email)).
					Param("$name").Text(testUserName).
					Build()))
			require.NoError(t, err)
			return id
		},
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

// newUser seeds a platform user with a fresh address on the backend and
// returns its id and email.
func newUser(t *testing.T, b backend) (id, email string) {
	t.Helper()
	addr := dbtest.RandString() + "@example.com"
	return b.insertUser(t, addr), addr
}

// createWorkspace creates a tenant with a fresh slug owned by ownerID and
// returns the persisted tenant.
func createWorkspace(t *testing.T, repo domain.TenantRepository, ownerID string) *domain.Tenant {
	t.Helper()
	tn, err := domain.NewTenant("Workspace", "ws-"+dbtest.RandString())
	require.NoError(t, err)
	created, err := repo.CreateWorkspace(context.Background(), tn, ownerID)
	require.NoError(t, err)
	return created
}

package adapters_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	authadapters "github.com/nikolaymatrosov/nvelope/internal/auth/adapters"
	authdomain "github.com/nikolaymatrosov/nvelope/internal/auth/domain"
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
	// users seeds platform users through the auth context's repository for
	// the same backend. On Postgres, memberships and invitations carry foreign
	// keys to platform_users, and ListMembers joins it on both backends.
	users func(t *testing.T) authdomain.UserRepository
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
		users: func(t *testing.T) authdomain.UserRepository {
			return authadapters.NewUsers(dbtest.AppPool(t))
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
		users: func(t *testing.T) authdomain.UserRepository {
			return authadapters.NewYDBUsers(dbtest.YDBMigrated(t))
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

// newUser seeds a platform user with a fresh address through the backend's
// auth repository and returns its id and email.
func newUser(t *testing.T, b backend) (id, email string) {
	t.Helper()
	addr := dbtest.RandString() + "@example.com"
	e, err := authdomain.NewEmail(addr)
	require.NoError(t, err)
	u, err := authdomain.NewUser(e, testUserName)
	require.NoError(t, err)
	created, err := b.users(t).Create(context.Background(), u, "hash")
	require.NoError(t, err)
	return created.ID(), addr
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

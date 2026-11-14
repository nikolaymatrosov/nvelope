package adapters_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

func TestTenantsCreateWorkspace(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.tenants(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		created := createWorkspace(t, repo, ownerID)
		require.NotEmpty(t, created.ID(), "the adapter assigns an id")
		require.Equal(t, "Workspace", created.Name())
		require.Equal(t, domain.StatusActive, created.Status())

		// The owner membership was created in the same transaction.
		role, err := repo.GetMembershipRole(ctx, ownerID, created.ID())
		require.NoError(t, err)
		require.Equal(t, domain.RoleOwner, role)

		// So was the initial settings row, named after the tenant.
		settings, err := b.settings(t).Get(ctx, created.ID())
		require.NoError(t, err)
		require.Equal(t, "Workspace", settings.DisplayName())
	})
}

func TestTenantsCreateRejectsDuplicateSlug(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.tenants(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		slug := "dup-" + dbtest.RandString()

		first, err := domain.NewTenant("First", slug)
		require.NoError(t, err)
		_, err = repo.CreateWorkspace(ctx, first, ownerID)
		require.NoError(t, err)

		second, err := domain.NewTenant("Second", slug)
		require.NoError(t, err)
		_, err = repo.CreateWorkspace(ctx, second, ownerID)
		require.ErrorIs(t, err, domain.ErrSlugTaken)
	})
}

func TestTenantsGetBySlugAndID(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.tenants(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		created := createWorkspace(t, repo, ownerID)

		bySlug, err := repo.GetBySlug(ctx, created.Slug().String())
		require.NoError(t, err)
		require.Equal(t, created, bySlug)

		// Slugs match ignoring case, as the citext column does on Postgres.
		byUpperSlug, err := repo.GetBySlug(ctx, strings.ToUpper(created.Slug().String()))
		require.NoError(t, err)
		require.Equal(t, created.ID(), byUpperSlug.ID())

		byID, err := repo.GetByID(ctx, created.ID())
		require.NoError(t, err)
		require.Equal(t, created, byID)

		_, err = repo.GetBySlug(ctx, "no-such-"+dbtest.RandString())
		require.ErrorIs(t, err, domain.ErrTenantNotFound)

		_, err = repo.GetByID(ctx, "not-a-uuid")
		require.ErrorIs(t, err, domain.ErrTenantNotFound, "a malformed id is an opaque not-found")
	})
}

func TestTenantsMembership(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.tenants(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		created := createWorkspace(t, repo, ownerID)
		stranger, _ := newUser(t, b)

		_, err := repo.GetMembershipRole(ctx, stranger, created.ID())
		require.ErrorIs(t, err, domain.ErrNotMember)

		m, err := domain.NewMembership(stranger, created.ID(), domain.RoleAdmin)
		require.NoError(t, err)
		require.NoError(t, repo.AddMembership(ctx, m))

		role, err := repo.GetMembershipRole(ctx, stranger, created.ID())
		require.NoError(t, err)
		require.Equal(t, domain.RoleAdmin, role)

		// Adding the same membership again is a harmless no-op.
		require.NoError(t, repo.AddMembership(ctx, m))
	})
}

// TestTenantsAddMembershipKeepsExistingRole pins Postgres's ON CONFLICT DO
// NOTHING: re-adding an existing member never changes their role.
func TestTenantsAddMembershipKeepsExistingRole(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.tenants(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		created := createWorkspace(t, repo, ownerID)

		m, err := domain.NewMembership(ownerID, created.ID(), domain.RoleAdmin)
		require.NoError(t, err)
		require.NoError(t, repo.AddMembership(ctx, m))

		role, err := repo.GetMembershipRole(ctx, ownerID, created.ID())
		require.NoError(t, err)
		require.Equal(t, domain.RoleOwner, role, "the owner is not demoted")
	})
}

func TestTenantsListMembers(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.tenants(t)
		ctx := context.Background()

		ownerID, ownerEmail := newUser(t, b)
		created := createWorkspace(t, repo, ownerID)
		adminID, adminEmail := newUser(t, b)
		m, err := domain.NewMembership(adminID, created.ID(), domain.RoleAdmin)
		require.NoError(t, err)
		require.NoError(t, repo.AddMembership(ctx, m))

		// Another workspace's owner is not a member of this one.
		outsiderID, _ := newUser(t, b)
		createWorkspace(t, repo, outsiderID)

		members, err := repo.ListMembers(ctx, created.ID())
		require.NoError(t, err)
		require.Equal(t, []domain.Member{
			{UserID: ownerID, Email: ownerEmail, Name: testUserName, Role: domain.RoleOwner},
			{UserID: adminID, Email: adminEmail, Name: testUserName, Role: domain.RoleAdmin},
		}, members, "only this tenant's members, oldest membership first")
	})
}

func TestTenantsListMembershipsForUser(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.tenants(t)
		ctx := context.Background()

		userID, _ := newUser(t, b)
		owned := createWorkspace(t, repo, userID)

		otherOwnerID, _ := newUser(t, b)
		joined := createWorkspace(t, repo, otherOwnerID)
		m, err := domain.NewMembership(userID, joined.ID(), domain.RoleAdmin)
		require.NoError(t, err)
		require.NoError(t, repo.AddMembership(ctx, m))

		memberships, err := repo.ListMembershipsForUser(ctx, userID)
		require.NoError(t, err)
		require.Equal(t, []domain.MembershipDetail{
			{Tenant: owned, Role: domain.RoleOwner},
			{Tenant: joined, Role: domain.RoleAdmin},
		}, memberships, "every tenant of the user with its role, oldest membership first")

		loner, _ := newUser(t, b)
		none, err := repo.ListMembershipsForUser(ctx, loner)
		require.NoError(t, err)
		require.Empty(t, none)
	})
}

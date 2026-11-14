package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

func TestUsersAddGetByPlatformUser(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.users(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		platformID := b.newPlatformUser(t)

		u, err := domain.NewTenantUser(tenantID, platformID, "user@example.com", "Pat")
		require.NoError(t, err)
		id, err := repo.Add(ctx, tenantID, u)
		require.NoError(t, err)
		require.NotEmpty(t, id, "the adapter assigns an id")

		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, id, got.ID())
		require.Equal(t, tenantID, got.TenantID())
		require.Equal(t, platformID, got.PlatformUserID())
		require.Equal(t, "user@example.com", got.Email())
		require.Equal(t, "Pat", got.Name())
		require.Equal(t, domain.UserActive, got.Status())
		require.False(t, got.TOTPEnabled())
		require.Nil(t, got.TOTPSecret())
		require.WithinDuration(t, time.Now(), got.CreatedAt(), time.Minute, "the database stamps the insert")

		byPlatform, err := repo.ByPlatformUser(ctx, tenantID, platformID)
		require.NoError(t, err)
		require.Equal(t, got, byPlatform)

		_, err = repo.ByPlatformUser(ctx, tenantID, "00000000-0000-0000-0000-000000000000")
		require.ErrorIs(t, err, domain.ErrUserNotFound)
		_, err = repo.Get(ctx, tenantID, "00000000-0000-0000-0000-000000000000")
		require.ErrorIs(t, err, domain.ErrUserNotFound)
		_, err = repo.Get(ctx, tenantID, "not-a-uuid")
		require.ErrorIs(t, err, domain.ErrUserNotFound, "a malformed id is an opaque not-found")
	})
}

// TestUsersAddRejectsDuplicates pins the two Postgres unique constraints:
// (tenant_id, platform_user_id) and (tenant_id, email), the email ignoring
// case as the citext column does.
func TestUsersAddRejectsDuplicates(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.users(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		platformID := b.newPlatformUser(t)

		first, err := domain.NewTenantUser(tenantID, platformID, "dup@example.com", "First")
		require.NoError(t, err)
		_, err = repo.Add(ctx, tenantID, first)
		require.NoError(t, err)

		samePlatformUser, err := domain.NewTenantUser(tenantID, platformID, "other@example.com", "Second")
		require.NoError(t, err)
		_, err = repo.Add(ctx, tenantID, samePlatformUser)
		require.Error(t, err, "a platform user has one tenant-plane user per tenant")

		sameEmail, err := domain.NewTenantUser(tenantID, b.newPlatformUser(t), "DUP@Example.com", "Third")
		require.NoError(t, err)
		_, err = repo.Add(ctx, tenantID, sameEmail)
		require.Error(t, err, "an email is unique within a tenant, ignoring case")

		otherTenant := b.newTenant(t)
		elsewhere, err := domain.NewTenantUser(otherTenant, platformID, "dup@example.com", "Elsewhere")
		require.NoError(t, err)
		_, err = repo.Add(ctx, otherTenant, elsewhere)
		require.NoError(t, err, "the same platform user and email may join another tenant")
	})
}

func TestUsersUpdateTOTP(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.users(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		u, err := domain.NewTenantUser(tenantID, b.newPlatformUser(t), "totp@example.com", "Pat")
		require.NoError(t, err)
		id, err := repo.Add(ctx, tenantID, u)
		require.NoError(t, err)

		require.NoError(t, repo.Update(ctx, tenantID, id, func(u *domain.TenantUser) (*domain.TenantUser, error) {
			return u, u.EnableTOTP([]byte("encrypted-secret"))
		}))
		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.True(t, got.TOTPEnabled())
		require.Equal(t, []byte("encrypted-secret"), got.TOTPSecret())

		require.NoError(t, repo.Update(ctx, tenantID, id, func(u *domain.TenantUser) (*domain.TenantUser, error) {
			u.DisableTOTP()
			u.Suspend()
			return u, nil
		}))
		got, err = repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.False(t, got.TOTPEnabled())
		require.Nil(t, got.TOTPSecret(), "disabling TOTP clears the stored secret")
		require.Equal(t, domain.UserSuspended, got.Status())
	})
}

func TestUsersUpdateRollsBackOnClosureError(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.users(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		id := newTenantUser(t, b, tenantID)

		sentinel := domain.ErrRoleNotFound
		err := repo.Update(ctx, tenantID, id, func(u *domain.TenantUser) (*domain.TenantUser, error) {
			u.Suspend()
			return nil, sentinel
		})
		require.ErrorIs(t, err, sentinel)

		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, domain.UserActive, got.Status(), "a closure error rolls the transaction back")
	})
}

func TestUsersAreTenantScoped(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.users(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		otherTenant := b.newTenant(t)
		platformID := b.newPlatformUser(t)

		u, err := domain.NewTenantUser(tenantID, platformID, "scoped@example.com", "Pat")
		require.NoError(t, err)
		id, err := repo.Add(ctx, tenantID, u)
		require.NoError(t, err)

		_, err = repo.Get(ctx, otherTenant, id)
		require.ErrorIs(t, err, domain.ErrUserNotFound)
		_, err = repo.ByPlatformUser(ctx, otherTenant, platformID)
		require.ErrorIs(t, err, domain.ErrUserNotFound)
		err = repo.Update(ctx, otherTenant, id, func(u *domain.TenantUser) (*domain.TenantUser, error) {
			u.Suspend()
			return u, nil
		})
		require.ErrorIs(t, err, domain.ErrUserNotFound)

		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, domain.UserActive, got.Status(), "another tenant cannot change the user")
	})
}

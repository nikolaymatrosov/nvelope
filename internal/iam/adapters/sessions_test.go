package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

func TestSessionsAddByTokenHash(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.sessions(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		userID := newTenantUser(t, b, tenantID)

		hash := "hash-" + dbtest.RandString()
		expiresAt := time.Now().Add(time.Hour)
		s, err := domain.NewSession(tenantID, userID, hash, false, expiresAt)
		require.NoError(t, err)
		id, err := repo.Add(ctx, tenantID, s)
		require.NoError(t, err)
		require.NotEmpty(t, id, "the adapter assigns an id")

		got, err := repo.ByTokenHash(ctx, tenantID, hash)
		require.NoError(t, err)
		require.Equal(t, id, got.ID())
		require.Equal(t, tenantID, got.TenantID())
		require.Equal(t, userID, got.UserID())
		require.Equal(t, hash, got.TokenHash())
		require.Equal(t, domain.SessionActive, got.State())
		require.WithinDuration(t, expiresAt, got.ExpiresAt(), time.Second)
		require.WithinDuration(t, time.Now(), got.CreatedAt(), time.Minute, "the database stamps the insert")
		require.Nil(t, got.RevokedAt())

		_, err = repo.ByTokenHash(ctx, tenantID, "no-such-hash")
		require.ErrorIs(t, err, domain.ErrSessionNotFound)
	})
}

func TestSessionsUpdateRevoke(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.sessions(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		userID := newTenantUser(t, b, tenantID)

		hash := "hash-" + dbtest.RandString()
		s, err := domain.NewSession(tenantID, userID, hash, true, time.Now().Add(time.Hour))
		require.NoError(t, err)
		id, err := repo.Add(ctx, tenantID, s)
		require.NoError(t, err)

		got, err := repo.ByTokenHash(ctx, tenantID, hash)
		require.NoError(t, err)
		require.Equal(t, domain.SessionTOTPPending, got.State())

		require.NoError(t, repo.Update(ctx, tenantID, id, func(s *domain.Session) (*domain.Session, error) {
			return s, s.CompleteTOTP()
		}))
		got, err = repo.ByTokenHash(ctx, tenantID, hash)
		require.NoError(t, err)
		require.Equal(t, domain.SessionActive, got.State())

		revokedAt := time.Now()
		require.NoError(t, repo.Update(ctx, tenantID, id, func(s *domain.Session) (*domain.Session, error) {
			s.Revoke(revokedAt)
			return s, nil
		}))
		got, err = repo.ByTokenHash(ctx, tenantID, hash)
		require.NoError(t, err)
		require.Equal(t, domain.SessionRevoked, got.State())
		require.NotNil(t, got.RevokedAt())
		require.WithinDuration(t, revokedAt, *got.RevokedAt(), time.Second)
	})
}

func TestSessionsAreTenantScoped(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.sessions(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		otherTenant := b.newTenant(t)
		userID := newTenantUser(t, b, tenantID)

		hash := "hash-" + dbtest.RandString()
		s, err := domain.NewSession(tenantID, userID, hash, false, time.Now().Add(time.Hour))
		require.NoError(t, err)
		id, err := repo.Add(ctx, tenantID, s)
		require.NoError(t, err)

		_, err = repo.ByTokenHash(ctx, otherTenant, hash)
		require.ErrorIs(t, err, domain.ErrSessionNotFound)
		revoke := func(s *domain.Session) (*domain.Session, error) {
			s.Revoke(time.Now())
			return s, nil
		}
		require.ErrorIs(t, repo.Update(ctx, otherTenant, id, revoke), domain.ErrSessionNotFound)
		require.ErrorIs(t, repo.Update(ctx, tenantID, "00000000-0000-0000-0000-000000000000", revoke),
			domain.ErrSessionNotFound)
		require.ErrorIs(t, repo.Update(ctx, tenantID, "not-a-uuid", revoke),
			domain.ErrSessionNotFound, "a malformed id is an opaque not-found")

		got, err := repo.ByTokenHash(ctx, tenantID, hash)
		require.NoError(t, err)
		require.Equal(t, domain.SessionActive, got.State(), "another tenant cannot revoke the session")
	})
}

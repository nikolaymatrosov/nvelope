package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/iam/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

func TestSessionsAddByTokenHash(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewSessions(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	userID := seedTenantUser(t, pool, tenantID)

	hash := "hash-" + dbtest.RandString()
	s, err := domain.NewSession(tenantID, userID, hash, false, time.Now().Add(time.Hour))
	require.NoError(t, err)
	_, err = repo.Add(ctx, tenantID, s)
	require.NoError(t, err)

	got, err := repo.ByTokenHash(ctx, tenantID, hash)
	require.NoError(t, err)
	require.Equal(t, domain.SessionActive, got.State())
	require.Equal(t, userID, got.UserID())

	_, err = repo.ByTokenHash(ctx, tenantID, "no-such-hash")
	require.ErrorIs(t, err, domain.ErrSessionNotFound)
}

func TestSessionsUpdateRevoke(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewSessions(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	userID := seedTenantUser(t, pool, tenantID)

	hash := "hash-" + dbtest.RandString()
	s, err := domain.NewSession(tenantID, userID, hash, true, time.Now().Add(time.Hour))
	require.NoError(t, err)
	id, err := repo.Add(ctx, tenantID, s)
	require.NoError(t, err)

	require.NoError(t, repo.Update(ctx, tenantID, id, func(s *domain.Session) (*domain.Session, error) {
		return s, s.CompleteTOTP()
	}))
	got, err := repo.ByTokenHash(ctx, tenantID, hash)
	require.NoError(t, err)
	require.Equal(t, domain.SessionActive, got.State())

	require.NoError(t, repo.Update(ctx, tenantID, id, func(s *domain.Session) (*domain.Session, error) {
		s.Revoke(time.Now())
		return s, nil
	}))
	got, err = repo.ByTokenHash(ctx, tenantID, hash)
	require.NoError(t, err)
	require.Equal(t, domain.SessionRevoked, got.State())
	require.NotNil(t, got.RevokedAt())
}

func TestAuditRecordAndAll(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewAudit(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	userID := seedTenantUser(t, pool, tenantID)

	rec := domain.NewAuditRecord(tenantID, userID, domain.PrincipalSession,
		"role.create", "role:Editor", map[string]any{"name": "Editor"})
	require.NoError(t, repo.Record(ctx, tenantID, rec))

	records, total, err := repo.All(ctx, tenantID, domain.Page{})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, "role.create", records[0].Action)
	require.Equal(t, "Editor", records[0].Metadata["name"])
}

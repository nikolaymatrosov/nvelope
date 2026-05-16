package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/tenant/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

func newInvitation(t *testing.T, tenantID, invitedBy string) *domain.Invitation {
	t.Helper()
	email, err := domain.NewEmail(dbtest.RandString() + "@example.com")
	require.NoError(t, err)
	inv, err := domain.NewInvitation(tenantID, email, invitedBy, time.Hour)
	require.NoError(t, err)
	return inv
}

func TestInvitationsCreateAndGet(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	tenants := adapters.NewTenants(pool)
	repo := adapters.NewInvitations(pool)
	ctx := context.Background()

	ownerID := insertUser(t, pool)
	ws := createWorkspace(t, tenants, ownerID)

	inv := newInvitation(t, ws.ID(), ownerID)
	tokenHash := dbtest.RandString()
	created, err := repo.Create(ctx, inv, tokenHash)
	require.NoError(t, err)
	require.NotEmpty(t, created.ID())
	require.Equal(t, domain.InvitationPending, created.Status())

	got, err := repo.GetPendingByTokenHash(ctx, tokenHash)
	require.NoError(t, err)
	require.Equal(t, created.ID(), got.ID())
	require.Equal(t, inv.Email().String(), got.Email().String())
}

func TestInvitationsCreateRejectsDuplicatePending(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	tenants := adapters.NewTenants(pool)
	repo := adapters.NewInvitations(pool)
	ctx := context.Background()

	ownerID := insertUser(t, pool)
	ws := createWorkspace(t, tenants, ownerID)

	email, err := domain.NewEmail(dbtest.RandString() + "@example.com")
	require.NoError(t, err)
	first, err := domain.NewInvitation(ws.ID(), email, ownerID, time.Hour)
	require.NoError(t, err)
	_, err = repo.Create(ctx, first, dbtest.RandString())
	require.NoError(t, err)

	second, err := domain.NewInvitation(ws.ID(), email, ownerID, time.Hour)
	require.NoError(t, err)
	_, err = repo.Create(ctx, second, dbtest.RandString())
	require.ErrorIs(t, err, domain.ErrInvitationExists)
}

func TestInvitationsGetPendingNotFound(t *testing.T) {
	t.Parallel()
	repo := adapters.NewInvitations(dbtest.AppPool(t))

	_, err := repo.GetPendingByTokenHash(context.Background(), dbtest.RandString())
	require.ErrorIs(t, err, domain.ErrInvitationNotFound)
}

func TestInvitationsUpdateAccept(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	tenants := adapters.NewTenants(pool)
	repo := adapters.NewInvitations(pool)
	ctx := context.Background()

	ownerID := insertUser(t, pool)
	ws := createWorkspace(t, tenants, ownerID)
	created, err := repo.Create(ctx, newInvitation(t, ws.ID(), ownerID), dbtest.RandString())
	require.NoError(t, err)

	err = repo.Update(ctx, created.ID(), ws.ID(), func(inv *domain.Invitation) (*domain.Invitation, error) {
		if err := inv.Accept(time.Now()); err != nil {
			return nil, err
		}
		return inv, nil
	})
	require.NoError(t, err)

	// The invitation is no longer pending, so a token lookup no longer finds it.
	err = repo.Update(ctx, created.ID(), ws.ID(), func(inv *domain.Invitation) (*domain.Invitation, error) {
		require.Equal(t, domain.InvitationAccepted, inv.Status())
		return inv, nil
	})
	require.NoError(t, err)
}

func TestInvitationsUpdateRevoke(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	tenants := adapters.NewTenants(pool)
	repo := adapters.NewInvitations(pool)
	ctx := context.Background()

	ownerID := insertUser(t, pool)
	ws := createWorkspace(t, tenants, ownerID)
	created, err := repo.Create(ctx, newInvitation(t, ws.ID(), ownerID), dbtest.RandString())
	require.NoError(t, err)

	err = repo.Update(ctx, created.ID(), ws.ID(), func(inv *domain.Invitation) (*domain.Invitation, error) {
		if err := inv.Revoke(); err != nil {
			return nil, err
		}
		return inv, nil
	})
	require.NoError(t, err)
}

func TestInvitationsUpdateNotFound(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	tenants := adapters.NewTenants(pool)
	repo := adapters.NewInvitations(pool)
	ctx := context.Background()

	ownerID := insertUser(t, pool)
	ws := createWorkspace(t, tenants, ownerID)

	noop := func(inv *domain.Invitation) (*domain.Invitation, error) { return inv, nil }

	err := repo.Update(ctx, "not-a-uuid", ws.ID(), noop)
	require.ErrorIs(t, err, domain.ErrInvitationNotFound, "a malformed id is an opaque not-found")
}

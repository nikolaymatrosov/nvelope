package tenant

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nvelope/nvelope/internal/dbtest"
)

func TestCreateAndGetInvitation(t *testing.T) {
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	owner := insertTestUser(t, pool)
	tn, err := CreateTenant(ctx, pool, owner, "Inviter", "inv-"+dbtest.RandString())
	require.NoError(t, err)

	email := dbtest.RandString() + "@example.com"
	inv, rawToken, err := CreateInvitation(ctx, pool, tn.ID, email, owner, time.Hour)
	require.NoError(t, err)
	require.Equal(t, "pending", inv.Status)
	require.NotEmpty(t, rawToken)

	found, err := GetPendingInvitationByToken(ctx, pool, rawToken)
	require.NoError(t, err)
	require.Equal(t, inv.ID, found.ID)
	require.Equal(t, email, found.Email)
}

func TestGetInvitationRejectsUnknownToken(t *testing.T) {
	pool := dbtest.AppPool(t)
	_, err := GetPendingInvitationByToken(context.Background(), pool, "not-a-real-token")
	require.ErrorIs(t, err, ErrInvitationNotFound)
}

func TestGetInvitationRejectsExpiredToken(t *testing.T) {
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	owner := insertTestUser(t, pool)
	tn, err := CreateTenant(ctx, pool, owner, "Inviter", "inv-"+dbtest.RandString())
	require.NoError(t, err)

	_, rawToken, err := CreateInvitation(ctx, pool, tn.ID,
		dbtest.RandString()+"@example.com", owner, -time.Minute)
	require.NoError(t, err)

	_, err = GetPendingInvitationByToken(ctx, pool, rawToken)
	require.ErrorIs(t, err, ErrInvitationNotFound, "an expired invitation does not resolve")
}

func TestCreateInvitationRejectsDuplicatePending(t *testing.T) {
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	owner := insertTestUser(t, pool)
	tn, err := CreateTenant(ctx, pool, owner, "Inviter", "inv-"+dbtest.RandString())
	require.NoError(t, err)

	email := dbtest.RandString() + "@example.com"
	_, _, err = CreateInvitation(ctx, pool, tn.ID, email, owner, time.Hour)
	require.NoError(t, err)
	_, _, err = CreateInvitation(ctx, pool, tn.ID, email, owner, time.Hour)
	require.ErrorIs(t, err, ErrInvitationExists)
}

func TestAcceptInvitationLifecycle(t *testing.T) {
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	owner := insertTestUser(t, pool)
	tn, err := CreateTenant(ctx, pool, owner, "Inviter", "inv-"+dbtest.RandString())
	require.NoError(t, err)

	inv, rawToken, err := CreateInvitation(ctx, pool, tn.ID,
		dbtest.RandString()+"@example.com", owner, time.Hour)
	require.NoError(t, err)

	accepted, err := MarkInvitationAccepted(ctx, pool, inv.ID, owner)
	require.NoError(t, err)
	require.True(t, accepted)

	// Once accepted, the token no longer resolves and cannot be accepted again.
	_, err = GetPendingInvitationByToken(ctx, pool, rawToken)
	require.ErrorIs(t, err, ErrInvitationNotFound)

	accepted, err = MarkInvitationAccepted(ctx, pool, inv.ID, owner)
	require.NoError(t, err)
	require.False(t, accepted, "an already-accepted invitation cannot be accepted again")
}

func TestRevokeInvitation(t *testing.T) {
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	owner := insertTestUser(t, pool)
	tn, err := CreateTenant(ctx, pool, owner, "Inviter", "inv-"+dbtest.RandString())
	require.NoError(t, err)

	inv, rawToken, err := CreateInvitation(ctx, pool, tn.ID,
		dbtest.RandString()+"@example.com", owner, time.Hour)
	require.NoError(t, err)

	require.NoError(t, RevokeInvitation(ctx, pool, tn.ID, inv.ID))

	_, err = GetPendingInvitationByToken(ctx, pool, rawToken)
	require.ErrorIs(t, err, ErrInvitationNotFound, "a revoked invitation does not resolve")

	// Revoking again, or revoking an unknown id, is reported as not found.
	require.ErrorIs(t, RevokeInvitation(ctx, pool, tn.ID, inv.ID), ErrInvitationNotFound)
	require.ErrorIs(t, RevokeInvitation(ctx, pool, tn.ID, "not-a-uuid"), ErrInvitationNotFound)
}

func TestListPendingInvitations(t *testing.T) {
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	owner := insertTestUser(t, pool)
	tn, err := CreateTenant(ctx, pool, owner, "Inviter", "inv-"+dbtest.RandString())
	require.NoError(t, err)

	for range 3 {
		_, _, err := CreateInvitation(ctx, pool, tn.ID,
			dbtest.RandString()+"@example.com", owner, time.Hour)
		require.NoError(t, err)
	}

	invitations, err := ListPendingInvitations(ctx, pool, tn.ID)
	require.NoError(t, err)
	require.Len(t, invitations, 3)
}

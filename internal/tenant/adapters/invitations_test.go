package adapters_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

func newInvitation(t *testing.T, tenantID, invitedBy string) *domain.Invitation {
	t.Helper()
	return inviteEmail(t, tenantID, invitedBy, dbtest.RandString()+"@example.com")
}

// inviteEmail builds a pending invitation for addr that expires in an hour.
func inviteEmail(t *testing.T, tenantID, invitedBy, addr string) *domain.Invitation {
	t.Helper()
	email, err := domain.NewEmail(addr)
	require.NoError(t, err)
	inv, err := domain.NewInvitation(tenantID, email, invitedBy, time.Hour)
	require.NoError(t, err)
	return inv
}

func accept(inv *domain.Invitation) (*domain.Invitation, error) {
	if err := inv.Accept(time.Now()); err != nil {
		return nil, err
	}
	return inv, nil
}

func revoke(inv *domain.Invitation) (*domain.Invitation, error) {
	if err := inv.Revoke(); err != nil {
		return nil, err
	}
	return inv, nil
}

func TestInvitationsCreateAndGet(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.invitations(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		ws := createWorkspace(t, b.tenants(t), ownerID)

		inv := newInvitation(t, ws.ID(), ownerID)
		tokenHash := dbtest.RandString()
		created, err := repo.Create(ctx, inv, tokenHash)
		require.NoError(t, err)
		require.NotEmpty(t, created.ID(), "the adapter assigns an id")
		require.Equal(t, domain.InvitationPending, created.Status())
		require.Equal(t, ws.ID(), created.TenantID())
		require.Equal(t, ownerID, created.InvitedBy())
		require.WithinDuration(t, time.Now(), created.CreatedAt(), time.Minute)

		got, err := repo.GetPendingByTokenHash(ctx, tokenHash)
		require.NoError(t, err)
		require.Equal(t, created.ID(), got.ID())
		require.Equal(t, ws.ID(), got.TenantID())
		require.Equal(t, inv.Email().String(), got.Email().String())
		require.Equal(t, ownerID, got.InvitedBy())
		require.WithinDuration(t, inv.ExpiresAt(), got.ExpiresAt(), time.Second)
	})
}

func TestInvitationsCreateRejectsDuplicatePending(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.invitations(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		ws := createWorkspace(t, b.tenants(t), ownerID)
		addr := dbtest.RandString() + "@example.com"

		_, err := repo.Create(ctx, inviteEmail(t, ws.ID(), ownerID, addr), dbtest.RandString())
		require.NoError(t, err)

		_, err = repo.Create(ctx, inviteEmail(t, ws.ID(), ownerID, addr), dbtest.RandString())
		require.ErrorIs(t, err, domain.ErrInvitationExists)
	})
}

// TestInvitationsEmailMatchesCaseInsensitively pins what the citext column
// gives Postgres. The pending-uniqueness check ignores case, and the address
// is returned as it was entered.
func TestInvitationsEmailMatchesCaseInsensitively(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.invitations(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		ws := createWorkspace(t, b.tenants(t), ownerID)

		mixed := "Bob." + dbtest.RandString() + "@Example.COM"
		tokenHash := dbtest.RandString()
		_, err := repo.Create(ctx, inviteEmail(t, ws.ID(), ownerID, mixed), tokenHash)
		require.NoError(t, err)

		got, err := repo.GetPendingByTokenHash(ctx, tokenHash)
		require.NoError(t, err)
		require.Equal(t, mixed, got.Email().String(), "the address is stored as entered")

		_, err = repo.Create(ctx, inviteEmail(t, ws.ID(), ownerID, strings.ToLower(mixed)), dbtest.RandString())
		require.ErrorIs(t, err, domain.ErrInvitationExists, "emails differing only in case collide")
	})
}

// TestInvitationsPendingUniquenessIsPerTenantAndStatus pins the scope of the
// Postgres partial unique index (tenant_id, email) WHERE status = 'pending'.
// Another tenant may invite the same address, and the address may be invited
// again once the earlier invitation is no longer pending.
func TestInvitationsPendingUniquenessIsPerTenantAndStatus(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.invitations(t)
		tenants := b.tenants(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		first := createWorkspace(t, tenants, ownerID)
		second := createWorkspace(t, tenants, ownerID)
		addr := dbtest.RandString() + "@example.com"

		pending, err := repo.Create(ctx, inviteEmail(t, first.ID(), ownerID, addr), dbtest.RandString())
		require.NoError(t, err)
		_, err = repo.Create(ctx, inviteEmail(t, second.ID(), ownerID, addr), dbtest.RandString())
		require.NoError(t, err, "another tenant may invite the same address")

		require.NoError(t, repo.Update(ctx, pending.ID(), first.ID(), revoke))
		_, err = repo.Create(ctx, inviteEmail(t, first.ID(), ownerID, addr), dbtest.RandString())
		require.NoError(t, err, "a revoked invitation no longer blocks a new one")
	})
}

// TestInvitationsCreateIsRaceFree pins the pending-uniqueness guarantee under
// concurrency. Postgres gets it from the partial unique index. YDB gets it from
// the serializable SELECT-then-INSERT, which the SDK retries on conflict.
func TestInvitationsCreateIsRaceFree(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.invitations(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		ws := createWorkspace(t, b.tenants(t), ownerID)
		addr := dbtest.RandString() + "@example.com"

		const n = 5
		errs := make([]error, n)
		var wg sync.WaitGroup
		for i := range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, errs[i] = repo.Create(ctx, inviteEmail(t, ws.ID(), ownerID, addr), dbtest.RandString())
			}()
		}
		wg.Wait()

		created := 0
		for _, err := range errs {
			if err == nil {
				created++
				continue
			}
			require.ErrorIs(t, err, domain.ErrInvitationExists)
		}
		require.Equal(t, 1, created, "exactly one concurrent invitation wins")

		pending, err := repo.ListPending(ctx, ws.ID())
		require.NoError(t, err)
		require.Len(t, pending, 1)
	})
}

func TestInvitationsGetPendingNotFound(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		_, err := b.invitations(t).GetPendingByTokenHash(context.Background(), dbtest.RandString())
		require.ErrorIs(t, err, domain.ErrInvitationNotFound)
	})
}

func TestInvitationsGetPendingRejectsExpired(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.invitations(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		ws := createWorkspace(t, b.tenants(t), ownerID)

		email, err := domain.NewEmail(dbtest.RandString() + "@example.com")
		require.NoError(t, err)
		expired, err := domain.NewInvitation(ws.ID(), email, ownerID, -time.Hour)
		require.NoError(t, err)
		tokenHash := dbtest.RandString()
		_, err = repo.Create(ctx, expired, tokenHash)
		require.NoError(t, err)

		_, err = repo.GetPendingByTokenHash(ctx, tokenHash)
		require.ErrorIs(t, err, domain.ErrInvitationNotFound, "an expired invitation does not resolve")
	})
}

func TestInvitationsListPending(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.invitations(t)
		tenants := b.tenants(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		ws := createWorkspace(t, tenants, ownerID)
		other := createWorkspace(t, tenants, ownerID)

		older, err := repo.Create(ctx, newInvitation(t, ws.ID(), ownerID), dbtest.RandString())
		require.NoError(t, err)
		newer, err := repo.Create(ctx, newInvitation(t, ws.ID(), ownerID), dbtest.RandString())
		require.NoError(t, err)
		revoked, err := repo.Create(ctx, newInvitation(t, ws.ID(), ownerID), dbtest.RandString())
		require.NoError(t, err)
		require.NoError(t, repo.Update(ctx, revoked.ID(), ws.ID(), revoke))
		_, err = repo.Create(ctx, newInvitation(t, other.ID(), ownerID), dbtest.RandString())
		require.NoError(t, err)

		pending, err := repo.ListPending(ctx, ws.ID())
		require.NoError(t, err)
		ids := make([]string, 0, len(pending))
		for _, inv := range pending {
			ids = append(ids, inv.ID())
		}
		require.Equal(t, []string{newer.ID(), older.ID()}, ids, "pending only, this tenant only, newest first")

		fresh := createWorkspace(t, tenants, ownerID)
		none, err := repo.ListPending(ctx, fresh.ID())
		require.NoError(t, err)
		require.Empty(t, none)
	})
}

func TestInvitationsUpdateAccept(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.invitations(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		ws := createWorkspace(t, b.tenants(t), ownerID)
		created, err := repo.Create(ctx, newInvitation(t, ws.ID(), ownerID), dbtest.RandString())
		require.NoError(t, err)

		require.NoError(t, repo.Update(ctx, created.ID(), ws.ID(), accept))

		// The persisted status is the accepted one.
		err = repo.Update(ctx, created.ID(), ws.ID(), func(inv *domain.Invitation) (*domain.Invitation, error) {
			require.Equal(t, domain.InvitationAccepted, inv.Status())
			return inv, nil
		})
		require.NoError(t, err)
	})
}

func TestInvitationsUpdateRevoke(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.invitations(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		ws := createWorkspace(t, b.tenants(t), ownerID)
		tokenHash := dbtest.RandString()
		created, err := repo.Create(ctx, newInvitation(t, ws.ID(), ownerID), tokenHash)
		require.NoError(t, err)

		require.NoError(t, repo.Update(ctx, created.ID(), ws.ID(), revoke))

		_, err = repo.GetPendingByTokenHash(ctx, tokenHash)
		require.ErrorIs(t, err, domain.ErrInvitationNotFound, "a revoked invitation does not resolve")
	})
}

func TestInvitationsUpdateNotFound(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.invitations(t)
		tenants := b.tenants(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		ws := createWorkspace(t, tenants, ownerID)

		noop := func(inv *domain.Invitation) (*domain.Invitation, error) { return inv, nil }

		err := repo.Update(ctx, "not-a-uuid", ws.ID(), noop)
		require.ErrorIs(t, err, domain.ErrInvitationNotFound, "a malformed id is an opaque not-found")

		created, err := repo.Create(ctx, newInvitation(t, ws.ID(), ownerID), dbtest.RandString())
		require.NoError(t, err)
		other := createWorkspace(t, tenants, ownerID)
		err = repo.Update(ctx, created.ID(), other.ID(), noop)
		require.ErrorIs(t, err, domain.ErrInvitationNotFound, "another tenant's invitation is not found")
	})
}

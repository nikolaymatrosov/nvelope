package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	authadapters "github.com/nikolaymatrosov/nvelope/internal/auth/adapters"
	authdomain "github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	tenantadapters "github.com/nikolaymatrosov/nvelope/internal/tenant/adapters"
	tenantcommand "github.com/nikolaymatrosov/nvelope/internal/tenant/app/command"
	tenantdomain "github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

const onboardingPassword = "correct-horse-battery"

// onboardingBackend is one implementation of tenantcommand.Onboarding under
// contract test, with the repositories of the same backend that seed the
// fixtures and check the outcome.
type onboardingBackend struct {
	name        string
	onboarding  func(t *testing.T) tenantcommand.Onboarding
	users       func(t *testing.T) authdomain.UserRepository
	sessions    func(t *testing.T) authdomain.SessionRepository
	tenants     func(t *testing.T) tenantdomain.TenantRepository
	invitations func(t *testing.T) tenantdomain.InvitationRepository
}

var onboardingBackends = []onboardingBackend{
	{
		name: "pg",
		onboarding: func(t *testing.T) tenantcommand.Onboarding {
			return newOnboarding(dbtest.AppPool(t), authadapters.NewPasswordHasher(), time.Hour)
		},
		users: func(t *testing.T) authdomain.UserRepository {
			return authadapters.NewUsers(dbtest.AppPool(t))
		},
		sessions: func(t *testing.T) authdomain.SessionRepository {
			return authadapters.NewSessions(dbtest.AppPool(t))
		},
		tenants: func(t *testing.T) tenantdomain.TenantRepository {
			return tenantadapters.NewTenants(dbtest.AppPool(t))
		},
		invitations: func(t *testing.T) tenantdomain.InvitationRepository {
			return tenantadapters.NewInvitations(dbtest.AppPool(t))
		},
	},
	{
		name: "ydb",
		onboarding: func(t *testing.T) tenantcommand.Onboarding {
			return newYDBOnboarding(dbtest.YDBMigrated(t), authadapters.NewPasswordHasher(), time.Hour)
		},
		users: func(t *testing.T) authdomain.UserRepository {
			return authadapters.NewYDBUsers(dbtest.YDBMigrated(t))
		},
		sessions: func(t *testing.T) authdomain.SessionRepository {
			return authadapters.NewYDBSessions(dbtest.YDBMigrated(t))
		},
		tenants: func(t *testing.T) tenantdomain.TenantRepository {
			return tenantadapters.NewYDBTenants(dbtest.YDBMigrated(t))
		},
		invitations: func(t *testing.T) tenantdomain.InvitationRepository {
			return tenantadapters.NewYDBInvitations(dbtest.YDBMigrated(t))
		},
	},
}

// eachOnboardingBackend runs body once per backend, as parallel subtests named
// after it.
func eachOnboardingBackend(t *testing.T, body func(t *testing.T, b onboardingBackend)) {
	t.Helper()
	for _, b := range onboardingBackends {
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()
			body(t, b)
		})
	}
}

// invited is a seeded workspace with one invitation.
type invited struct {
	tenantID     string
	ownerID      string
	invitationID string
	tokenHash    string
	email        string
}

// seedInvitation creates a workspace owner, their workspace and an invitation
// for a fresh address that expires ttl from now.
func seedInvitation(t *testing.T, b onboardingBackend, ttl time.Duration) invited {
	t.Helper()
	ctx := context.Background()
	ownerID := seedUser(t, b, dbtest.RandString()+"@example.com")

	tn, err := tenantdomain.NewTenant("Workspace", "ws-"+dbtest.RandString())
	require.NoError(t, err)
	ws, err := b.tenants(t).CreateWorkspace(ctx, tn, ownerID)
	require.NoError(t, err)

	addr := "invitee." + dbtest.RandString() + "@example.com"
	email, err := tenantdomain.NewEmail(addr)
	require.NoError(t, err)
	inv, err := tenantdomain.NewInvitation(ws.ID(), email, ownerID, ttl)
	require.NoError(t, err)
	tokenHash := dbtest.RandString()
	created, err := b.invitations(t).Create(ctx, inv, tokenHash)
	require.NoError(t, err)

	return invited{
		tenantID: ws.ID(), ownerID: ownerID, invitationID: created.ID(),
		tokenHash: tokenHash, email: addr,
	}
}

// seedUser registers a platform user with the address and returns its id.
func seedUser(t *testing.T, b onboardingBackend, addr string) string {
	t.Helper()
	email, err := authdomain.NewEmail(addr)
	require.NoError(t, err)
	u, err := authdomain.NewUser(email, "Existing User")
	require.NoError(t, err)
	created, err := b.users(t).Create(context.Background(), u, "hash")
	require.NoError(t, err)
	return created.ID()
}

func newUserAcceptance(inv invited) tenantcommand.NewUserAcceptance {
	return tenantcommand.NewUserAcceptance{
		InvitationID: inv.invitationID,
		TenantID:     inv.tenantID,
		Email:        inv.email,
		Password:     onboardingPassword,
		Name:         "Ada",
	}
}

func TestOnboardingAcceptForNewUser(t *testing.T) {
	t.Parallel()
	eachOnboardingBackend(t, func(t *testing.T, b onboardingBackend) {
		ctx := context.Background()
		inv := seedInvitation(t, b, time.Hour)

		got, err := b.onboarding(t).AcceptForNewUser(ctx, newUserAcceptance(inv))
		require.NoError(t, err)
		require.NotEmpty(t, got.ID)
		require.Equal(t, inv.email, got.Email)
		require.Equal(t, "Ada", got.Name)
		require.NotEmpty(t, got.SessionToken)

		// The account exists, with the password hashed.
		user, hash, err := b.users(t).GetCredentials(ctx, inv.email)
		require.NoError(t, err)
		require.Equal(t, got.ID, user.ID())
		require.True(t, authadapters.NewPasswordHasher().Verify(hash, onboardingPassword))

		// The returned token resolves to a live session for the new account.
		session, err := b.sessions(t).ResolveByTokenHash(ctx, token.Hash(got.SessionToken))
		require.NoError(t, err)
		require.Equal(t, got.ID, session.UserID())

		// The account joined as an admin, and the invitation is spent.
		role, err := b.tenants(t).GetMembershipRole(ctx, got.ID, inv.tenantID)
		require.NoError(t, err)
		require.Equal(t, tenantdomain.RoleAdmin, role)
		_, err = b.invitations(t).GetPendingByTokenHash(ctx, inv.tokenHash)
		require.ErrorIs(t, err, tenantdomain.ErrInvitationNotFound)
	})
}

// TestOnboardingAcceptForNewUserRejectsTakenEmail seeds the existing account
// with the invitee's address in upper case. The collision therefore depends on
// the case-insensitive email key, which citext gives Postgres and the shared
// InsertYDBUser gives YDB.
func TestOnboardingAcceptForNewUserRejectsTakenEmail(t *testing.T) {
	t.Parallel()
	eachOnboardingBackend(t, func(t *testing.T, b onboardingBackend) {
		ctx := context.Background()
		inv := seedInvitation(t, b, time.Hour)
		seedUser(t, b, strings.ToUpper(inv.email))

		_, err := b.onboarding(t).AcceptForNewUser(ctx, newUserAcceptance(inv))
		require.ErrorIs(t, err, authdomain.ErrEmailTaken)

		// The transaction rolled back, so the invitation is still pending.
		_, err = b.invitations(t).GetPendingByTokenHash(ctx, inv.tokenHash)
		require.NoError(t, err)
	})
}

func TestOnboardingAcceptForNewUserRollsBackOnSpentInvitation(t *testing.T) {
	t.Parallel()
	eachOnboardingBackend(t, func(t *testing.T, b onboardingBackend) {
		ctx := context.Background()
		inv := seedInvitation(t, b, time.Hour)
		err := b.invitations(t).Update(ctx, inv.invitationID, inv.tenantID,
			func(i *tenantdomain.Invitation) (*tenantdomain.Invitation, error) {
				if err := i.Revoke(); err != nil {
					return nil, err
				}
				return i, nil
			})
		require.NoError(t, err)

		_, err = b.onboarding(t).AcceptForNewUser(ctx, newUserAcceptance(inv))
		require.ErrorIs(t, err, tenantdomain.ErrInvitationNotFound)

		// The account insert rolled back with the failed acceptance.
		_, err = b.users(t).LookupByEmail(ctx, inv.email)
		require.ErrorIs(t, err, authdomain.ErrUserNotFound)
	})
}

func TestOnboardingAcceptForExistingUser(t *testing.T) {
	t.Parallel()
	eachOnboardingBackend(t, func(t *testing.T, b onboardingBackend) {
		ctx := context.Background()
		inv := seedInvitation(t, b, time.Hour)
		userID := seedUser(t, b, inv.email)
		acceptance := tenantcommand.ExistingUserAcceptance{
			InvitationID: inv.invitationID, TenantID: inv.tenantID, UserID: userID,
		}

		require.NoError(t, b.onboarding(t).AcceptForExistingUser(ctx, acceptance))

		role, err := b.tenants(t).GetMembershipRole(ctx, userID, inv.tenantID)
		require.NoError(t, err)
		require.Equal(t, tenantdomain.RoleAdmin, role)
		_, err = b.invitations(t).GetPendingByTokenHash(ctx, inv.tokenHash)
		require.ErrorIs(t, err, tenantdomain.ErrInvitationNotFound)

		// A spent invitation cannot be accepted again.
		err = b.onboarding(t).AcceptForExistingUser(ctx, acceptance)
		require.ErrorIs(t, err, tenantdomain.ErrInvitationNotFound)
	})
}

// TestOnboardingAcceptForExistingMemberKeepsRole pins the ON CONFLICT DO
// NOTHING of the Postgres membership insert: accepting never demotes a member.
func TestOnboardingAcceptForExistingMemberKeepsRole(t *testing.T) {
	t.Parallel()
	eachOnboardingBackend(t, func(t *testing.T, b onboardingBackend) {
		ctx := context.Background()
		inv := seedInvitation(t, b, time.Hour)

		require.NoError(t, b.onboarding(t).AcceptForExistingUser(ctx, tenantcommand.ExistingUserAcceptance{
			InvitationID: inv.invitationID, TenantID: inv.tenantID, UserID: inv.ownerID,
		}))

		role, err := b.tenants(t).GetMembershipRole(ctx, inv.ownerID, inv.tenantID)
		require.NoError(t, err)
		require.Equal(t, tenantdomain.RoleOwner, role, "the owner is not demoted")
	})
}

func TestOnboardingAcceptForExistingUserRejectsExpired(t *testing.T) {
	t.Parallel()
	eachOnboardingBackend(t, func(t *testing.T, b onboardingBackend) {
		ctx := context.Background()
		inv := seedInvitation(t, b, -time.Hour)
		userID := seedUser(t, b, dbtest.RandString()+"@example.com")

		err := b.onboarding(t).AcceptForExistingUser(ctx, tenantcommand.ExistingUserAcceptance{
			InvitationID: inv.invitationID, TenantID: inv.tenantID, UserID: userID,
		})
		require.ErrorIs(t, err, tenantdomain.ErrInvitationNotFound)

		_, err = b.tenants(t).GetMembershipRole(ctx, userID, inv.tenantID)
		require.ErrorIs(t, err, tenantdomain.ErrNotMember)
	})
}

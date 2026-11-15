package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	authadapters "github.com/nikolaymatrosov/nvelope/internal/auth/adapters"
	authcommand "github.com/nikolaymatrosov/nvelope/internal/auth/app/command"
	authdomain "github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/db"
	tenantadapters "github.com/nikolaymatrosov/nvelope/internal/tenant/adapters"
	tenantcommand "github.com/nikolaymatrosov/nvelope/internal/tenant/app/command"
	tenantdomain "github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// ydbOnboarding is the serverless (YDB) implementation of
// tenant/app/command.Onboarding, the counterpart of onboarding. Each operation
// is one serializable DoTx. Every row is written through a helper that the
// owning context exports:
//   - auth: InsertYDBUser and InsertYDBSession;
//   - tenant: AcceptYDBInvitation and AddYDBMembership.
//
// The email_key derivation and every table's SQL therefore have one source.
type ydbOnboarding struct {
	driver     *ydb.Driver
	hasher     authcommand.PasswordHasher
	sessionTTL time.Duration
}

var _ tenantcommand.Onboarding = ydbOnboarding{}

func newYDBOnboarding(d *ydb.Driver, hasher authcommand.PasswordHasher, sessionTTL time.Duration) ydbOnboarding {
	return ydbOnboarding{driver: d, hasher: hasher, sessionTTL: sessionTTL}
}

// AcceptForExistingUser marks the invitation accepted and records the
// membership for an already-registered user, in one transaction. A user who
// is already a member keeps their role.
func (o ydbOnboarding) AcceptForExistingUser(ctx context.Context, p tenantcommand.ExistingUserAcceptance) error {
	membership, err := tenantdomain.NewMembership(p.UserID, p.TenantID, tenantdomain.RoleAdmin)
	if err != nil {
		return err
	}
	// No IsYDBConflict -> nil mapping follows DoTx, on purpose: a conflict
	// rolls the invitation acceptance back too, so reporting success would leave
	// the invitation pending. Unlike the standalone YDBTenants.AddMembership,
	// the error must surface.
	return o.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		if err := tenantadapters.AcceptYDBInvitation(ctx, tx, p.TenantID, p.InvitationID, p.UserID); err != nil {
			return err
		}
		return tenantadapters.AddYDBMembership(ctx, tx, membership)
	})
}

// AcceptForNewUser creates the account, issues a session, marks the invitation
// accepted, and records the membership for a new user, all in one
// transaction. The user id, token, token hash, password hash and expiry are
// fixed before DoTx. The session row id is generated per attempt, which is
// harmless: a retried attempt is rolled back, the id is never returned, and
// token_hash is the key. The closure runs only database statements, so the
// SDK can re-run it safely.
func (o ydbOnboarding) AcceptForNewUser(ctx context.Context, p tenantcommand.NewUserAcceptance) (tenantcommand.NewlyOnboardedUser, error) {
	email, err := authdomain.NewEmail(p.Email)
	if err != nil {
		return tenantcommand.NewlyOnboardedUser{}, err
	}
	if _, err := authdomain.NewPassword(p.Password); err != nil {
		return tenantcommand.NewlyOnboardedUser{}, err
	}
	user, err := authdomain.NewUser(email, p.Name)
	if err != nil {
		return tenantcommand.NewlyOnboardedUser{}, err
	}
	hash, err := o.hasher.Hash(p.Password)
	if err != nil {
		return tenantcommand.NewlyOnboardedUser{}, fmt.Errorf("hashing password: %w", err)
	}
	rawToken, err := token.New()
	if err != nil {
		return tenantcommand.NewlyOnboardedUser{}, err
	}
	userID := uuid.NewString()
	session, err := authdomain.NewSession(userID, o.sessionTTL)
	if err != nil {
		return tenantcommand.NewlyOnboardedUser{}, err
	}
	membership, err := tenantdomain.NewMembership(userID, p.TenantID, tenantdomain.RoleAdmin)
	if err != nil {
		return tenantcommand.NewlyOnboardedUser{}, err
	}

	err = o.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		if err := authadapters.InsertYDBUser(ctx, tx, userID, user, hash); err != nil {
			return err
		}
		if err := authadapters.InsertYDBSession(ctx, tx, session, token.Hash(rawToken)); err != nil {
			return err
		}
		if err := tenantadapters.AcceptYDBInvitation(ctx, tx, p.TenantID, p.InvitationID, userID); err != nil {
			return err
		}
		return tenantadapters.AddYDBMembership(ctx, tx, membership)
	})
	// The spike saw the conflict at the statement; this covers a YDB that
	// reports it at commit. Only the email_key index can collide: every other
	// key this transaction writes is freshly generated.
	if db.IsYDBConflict(err) {
		return tenantcommand.NewlyOnboardedUser{}, authdomain.ErrEmailTaken
	}
	if err != nil {
		return tenantcommand.NewlyOnboardedUser{}, err
	}
	return tenantcommand.NewlyOnboardedUser{
		ID:           userID,
		Email:        email.String(),
		Name:         user.Name(),
		SessionToken: rawToken,
	}, nil
}

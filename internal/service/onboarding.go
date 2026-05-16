package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	authcommand "github.com/nikolaymatrosov/nvelope/internal/auth/app/command"
	authdomain "github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/db"
	tenantcommand "github.com/nikolaymatrosov/nvelope/internal/tenant/app/command"
	tenantdomain "github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// onboarding implements tenant/app/command.Onboarding. Accepting an invitation
// spans the auth schema (account, session) and the tenant schema (invitation
// status, membership); this implementation lives at the composition root
// because it is the one operation that must run a single transaction across
// both bounded contexts.
type onboarding struct {
	pool       *pgxpool.Pool
	hasher     authcommand.PasswordHasher
	sessionTTL time.Duration
}

func newOnboarding(pool *pgxpool.Pool, hasher authcommand.PasswordHasher, sessionTTL time.Duration) onboarding {
	return onboarding{pool: pool, hasher: hasher, sessionTTL: sessionTTL}
}

// AcceptForExistingUser marks the invitation accepted and records the
// membership for an already-registered user, in one transaction.
func (o onboarding) AcceptForExistingUser(ctx context.Context, p tenantcommand.ExistingUserAcceptance) error {
	return pgx.BeginFunc(ctx, o.pool, func(tx pgx.Tx) error {
		if err := acceptInvitationRow(ctx, tx, p.InvitationID, p.UserID); err != nil {
			return err
		}
		return addMembershipRow(ctx, tx, p.UserID, p.TenantID)
	})
}

// AcceptForNewUser creates the account, issues a session, marks the invitation
// accepted, and records the membership for a new user, all in one transaction.
func (o onboarding) AcceptForNewUser(ctx context.Context, p tenantcommand.NewUserAcceptance) (tenantcommand.NewlyOnboardedUser, error) {
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

	var newUserID string
	err = pgx.BeginFunc(ctx, o.pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`INSERT INTO platform_users (email, password_hash, name)
			 VALUES ($1, $2, $3) RETURNING id`,
			email.String(), hash, user.Name()).Scan(&newUserID)
		if err != nil {
			if db.IsUniqueViolation(err) {
				return authdomain.ErrEmailTaken
			}
			return fmt.Errorf("inserting platform user: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO platform_sessions (platform_user_id, token_hash, expires_at)
			 VALUES ($1, $2, $3)`,
			newUserID, token.Hash(rawToken), time.Now().Add(o.sessionTTL)); err != nil {
			return fmt.Errorf("inserting session: %w", err)
		}
		if err := acceptInvitationRow(ctx, tx, p.InvitationID, newUserID); err != nil {
			return err
		}
		return addMembershipRow(ctx, tx, newUserID, p.TenantID)
	})
	if err != nil {
		return tenantcommand.NewlyOnboardedUser{}, err
	}
	return tenantcommand.NewlyOnboardedUser{
		ID:           newUserID,
		Email:        email.String(),
		Name:         user.Name(),
		SessionToken: rawToken,
	}, nil
}

// acceptInvitationRow marks a pending, unexpired invitation accepted. The WHERE
// guard makes the transition atomic with the rest of the transaction: when the
// invitation is no longer acceptable no row matches and the whole transaction
// is rolled back with the opaque not-found error.
func acceptInvitationRow(ctx context.Context, tx pgx.Tx, invitationID, acceptedBy string) error {
	tag, err := tx.Exec(ctx,
		`UPDATE invitations
		 SET status = 'accepted', accepted_by = $1, accepted_at = now()
		 WHERE id = $2 AND status = 'pending' AND expires_at > now()`,
		acceptedBy, invitationID)
	if err != nil {
		return fmt.Errorf("accepting invitation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return tenantdomain.ErrInvitationNotFound
	}
	return nil
}

// addMembershipRow records an admin membership. Re-adding is a no-op.
func addMembershipRow(ctx context.Context, tx pgx.Tx, userID, tenantID string) error {
	if _, err := tx.Exec(ctx,
		`INSERT INTO platform_user_tenants (platform_user_id, tenant_id, role)
		 VALUES ($1, $2, 'admin')
		 ON CONFLICT (platform_user_id, tenant_id) DO NOTHING`,
		userID, tenantID); err != nil {
		return fmt.Errorf("inserting membership: %w", err)
	}
	return nil
}

// memberDirectory implements tenant/app/command.MemberDirectory. The lookup
// crosses the auth context (find a user by email) and the tenant context
// (check membership), so it is composed here.
type memberDirectory struct {
	users   authdomain.UserRepository
	tenants tenantdomain.TenantRepository
}

func newMemberDirectory(users authdomain.UserRepository, tenants tenantdomain.TenantRepository) memberDirectory {
	return memberDirectory{users: users, tenants: tenants}
}

// IsMember reports whether email already belongs to a member of the tenant.
func (d memberDirectory) IsMember(ctx context.Context, tenantID, email string) (bool, error) {
	user, err := d.users.LookupByEmail(ctx, email)
	if errors.Is(err, authdomain.ErrUserNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := d.tenants.GetMembershipRole(ctx, user.ID(), tenantID); err != nil {
		if errors.Is(err, tenantdomain.ErrNotMember) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

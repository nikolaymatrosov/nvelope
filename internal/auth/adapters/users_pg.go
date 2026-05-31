package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/db"
)

// Users is the pgx-backed implementation of domain.UserRepository.
type Users struct {
	pool *pgxpool.Pool
}

var _ domain.UserRepository = (*Users)(nil)

// NewUsers builds a Users repository over the given pool.
func NewUsers(pool *pgxpool.Pool) *Users {
	return &Users{pool: pool}
}

// Create inserts a new user with an already-hashed password and returns the
// persisted user with its database-assigned id.
func (r *Users) Create(ctx context.Context, u *domain.User, passwordHash string) (*domain.User, error) {
	var id, email, name string
	err := r.pool.QueryRow(ctx,
		`INSERT INTO platform_users (email, password_hash, name)
		 VALUES ($1, $2, $3)
		 RETURNING id, email, name`,
		u.Email().String(), passwordHash, u.Name()).Scan(&id, &email, &name)
	if err != nil {
		if db.IsUniqueViolation(err) {
			return nil, domain.ErrEmailTaken
		}
		return nil, fmt.Errorf("inserting platform user: %w", err)
	}
	// A freshly inserted user has no locale yet (NULL column) and is unverified.
	return domain.HydrateUser(id, email, name, "", nil), nil
}

// CreateWithSession atomically inserts a new user and issues an initial
// session. The user insert, the issueSession callback, and the session insert
// all run in one transaction.
func (r *Users) CreateWithSession(ctx context.Context, u *domain.User, passwordHash string,
	issueSession func(userID string) (*domain.Session, string, error)) (*domain.User, error) {

	var created *domain.User
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		var id, email, name string
		err := tx.QueryRow(ctx,
			`INSERT INTO platform_users (email, password_hash, name)
			 VALUES ($1, $2, $3)
			 RETURNING id, email, name`,
			u.Email().String(), passwordHash, u.Name()).Scan(&id, &email, &name)
		if err != nil {
			if db.IsUniqueViolation(err) {
				return domain.ErrEmailTaken
			}
			return fmt.Errorf("inserting platform user: %w", err)
		}

		session, tokenHash, err := issueSession(id)
		if err != nil {
			return err
		}
		if err := insertSession(ctx, tx, session, tokenHash); err != nil {
			return err
		}

		created = domain.HydrateUser(id, email, name, "", nil)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// CreateWithVerification atomically inserts a new user and its first
// email-verification challenge. The user insert, the issueVerification
// callback, and the token insert all run in one transaction.
func (r *Users) CreateWithVerification(ctx context.Context, u *domain.User, passwordHash string,
	issueVerification func(userID string) (*domain.EmailVerification, string, error)) (*domain.User, error) {

	var created *domain.User
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		var id, email, name string
		err := tx.QueryRow(ctx,
			`INSERT INTO platform_users (email, password_hash, name)
			 VALUES ($1, $2, $3)
			 RETURNING id, email, name`,
			u.Email().String(), passwordHash, u.Name()).Scan(&id, &email, &name)
		if err != nil {
			if db.IsUniqueViolation(err) {
				return domain.ErrEmailTaken
			}
			return fmt.Errorf("inserting platform user: %w", err)
		}

		verification, tokenHash, err := issueVerification(id)
		if err != nil {
			return err
		}
		if _, err := insertVerificationToken(ctx, tx, verification, tokenHash); err != nil {
			return err
		}

		// A freshly inserted user has no locale yet and is unverified.
		created = domain.HydrateUser(id, email, name, "", nil)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// MarkEmailVerified records that the user verified their email address at now.
// It is idempotent: the UPDATE only touches a still-unverified row, so a repeat
// call leaves the original instant untouched.
func (r *Users) MarkEmailVerified(ctx context.Context, userID string, now time.Time) error {
	if _, err := r.pool.Exec(ctx,
		`UPDATE platform_users SET email_verified_at = $1, updated_at = now()
		 WHERE id = $2 AND email_verified_at IS NULL`,
		now, userID); err != nil {
		return fmt.Errorf("marking email verified: %w", err)
	}
	return nil
}

// GetByID returns the user with the given id, or domain.ErrUserNotFound.
func (r *Users) GetByID(ctx context.Context, id string) (*domain.User, error) {
	var gotID, email, name, locale string
	var emailVerifiedAt *time.Time
	err := r.pool.QueryRow(ctx,
		"SELECT id, email, name, COALESCE(locale, ''), email_verified_at FROM platform_users WHERE id = $1", id).
		Scan(&gotID, &email, &name, &locale, &emailVerifiedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading platform user: %w", err)
	}
	return domain.HydrateUser(gotID, email, name, locale, emailVerifiedAt), nil
}

// UpdateLocale persists the user's interface-language preference. It returns
// domain.ErrUserNotFound when no user has the given id.
func (r *Users) UpdateLocale(ctx context.Context, userID string, locale domain.Locale) error {
	tag, err := r.pool.Exec(ctx,
		"UPDATE platform_users SET locale = $1, updated_at = now() WHERE id = $2",
		locale.String(), userID)
	if err != nil {
		return fmt.Errorf("updating user locale: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrUserNotFound
	}
	return nil
}

// LookupByEmail returns the user with the given email, or
// domain.ErrUserNotFound.
func (r *Users) LookupByEmail(ctx context.Context, email string) (*domain.User, error) {
	var id, gotEmail, name, locale string
	var emailVerifiedAt *time.Time
	err := r.pool.QueryRow(ctx,
		"SELECT id, email, name, COALESCE(locale, ''), email_verified_at FROM platform_users WHERE email = $1", email).
		Scan(&id, &gotEmail, &name, &locale, &emailVerifiedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("looking up user: %w", err)
	}
	return domain.HydrateUser(id, gotEmail, name, locale, emailVerifiedAt), nil
}

// GetCredentials returns the user and the stored bcrypt hash for an email, or
// domain.ErrUserNotFound. The hash never leaves the adapter/app boundary.
func (r *Users) GetCredentials(ctx context.Context, email string) (*domain.User, string, error) {
	var id, gotEmail, name, locale, hash string
	var emailVerifiedAt *time.Time
	err := r.pool.QueryRow(ctx,
		"SELECT id, email, name, COALESCE(locale, ''), email_verified_at, password_hash FROM platform_users WHERE email = $1", email).
		Scan(&id, &gotEmail, &name, &locale, &emailVerifiedAt, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", domain.ErrUserNotFound
	}
	if err != nil {
		return nil, "", fmt.Errorf("loading credentials: %w", err)
	}
	return domain.HydrateUser(id, gotEmail, name, locale, emailVerifiedAt), hash, nil
}

package adapters

import (
	"context"
	"errors"
	"fmt"

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
	return domain.HydrateUser(id, email, name), nil
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

		created = domain.HydrateUser(id, email, name)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// GetByID returns the user with the given id, or domain.ErrUserNotFound.
func (r *Users) GetByID(ctx context.Context, id string) (*domain.User, error) {
	var gotID, email, name string
	err := r.pool.QueryRow(ctx,
		"SELECT id, email, name FROM platform_users WHERE id = $1", id).
		Scan(&gotID, &email, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading platform user: %w", err)
	}
	return domain.HydrateUser(gotID, email, name), nil
}

// LookupByEmail returns the user with the given email, or
// domain.ErrUserNotFound.
func (r *Users) LookupByEmail(ctx context.Context, email string) (*domain.User, error) {
	var id, gotEmail, name string
	err := r.pool.QueryRow(ctx,
		"SELECT id, email, name FROM platform_users WHERE email = $1", email).
		Scan(&id, &gotEmail, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("looking up user: %w", err)
	}
	return domain.HydrateUser(id, gotEmail, name), nil
}

// GetCredentials returns the user and the stored bcrypt hash for an email, or
// domain.ErrUserNotFound. The hash never leaves the adapter/app boundary.
func (r *Users) GetCredentials(ctx context.Context, email string) (*domain.User, string, error) {
	var id, gotEmail, name, hash string
	err := r.pool.QueryRow(ctx,
		"SELECT id, email, name, password_hash FROM platform_users WHERE email = $1", email).
		Scan(&id, &gotEmail, &name, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", domain.ErrUserNotFound
	}
	if err != nil {
		return nil, "", fmt.Errorf("loading credentials: %w", err)
	}
	return domain.HydrateUser(id, gotEmail, name), hash, nil
}

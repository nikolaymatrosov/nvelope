package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/nvelope/nvelope/internal/db"
)

// User is a platform identity. It never carries the password hash.
type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// ErrEmailTaken is returned when creating a user whose email already exists.
var ErrEmailTaken = errors.New("email already registered")

// ErrUserNotFound is returned when no user matches a lookup.
var ErrUserNotFound = errors.New("user not found")

// CreateUser inserts a platform user. The caller supplies an already-hashed
// password. It returns ErrEmailTaken when the email is already registered.
func CreateUser(ctx context.Context, q db.Querier, email, passwordHash, name string) (User, error) {
	var u User
	err := q.QueryRow(ctx,
		`INSERT INTO platform_users (email, password_hash, name)
		 VALUES ($1, $2, $3)
		 RETURNING id, email, name`,
		email, passwordHash, name).Scan(&u.ID, &u.Email, &u.Name)
	if err != nil {
		if db.IsUniqueViolation(err) {
			return User{}, ErrEmailTaken
		}
		return User{}, fmt.Errorf("inserting platform user: %w", err)
	}
	return u, nil
}

// GetUserByID returns the user with the given id, or ErrUserNotFound.
func GetUserByID(ctx context.Context, q db.Querier, id string) (User, error) {
	var u User
	err := q.QueryRow(ctx,
		"SELECT id, email, name FROM platform_users WHERE id = $1", id).
		Scan(&u.ID, &u.Email, &u.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("loading platform user: %w", err)
	}
	return u, nil
}

// LookupUserByEmail returns the user with the given email, or ErrUserNotFound.
func LookupUserByEmail(ctx context.Context, q db.Querier, email string) (User, error) {
	var u User
	err := q.QueryRow(ctx,
		"SELECT id, email, name FROM platform_users WHERE email = $1", email).
		Scan(&u.ID, &u.Email, &u.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("looking up user: %w", err)
	}
	return u, nil
}

// getCredentials returns the user and their password hash for an email
// lookup, or ErrUserNotFound. It is unexported because the hash must not
// leave this package.
func getCredentials(ctx context.Context, q db.Querier, email string) (User, string, error) {
	var u User
	var hash string
	err := q.QueryRow(ctx,
		"SELECT id, email, name, password_hash FROM platform_users WHERE email = $1", email).
		Scan(&u.ID, &u.Email, &u.Name, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, "", ErrUserNotFound
	}
	if err != nil {
		return User{}, "", fmt.Errorf("loading credentials: %w", err)
	}
	return u, hash, nil
}

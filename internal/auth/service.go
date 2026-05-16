package auth

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nvelope/nvelope/internal/db"
)

// ErrInvalidCredentials is returned by Login for an unknown email or a wrong
// password. It is deliberately identical for both cases to resist account
// enumeration.
var ErrInvalidCredentials = errors.New("invalid email or password")

// ValidationError describes a request that failed input validation. Its
// message is safe to show the user.
type ValidationError struct{ Message string }

func (e ValidationError) Error() string { return e.Message }

const (
	minPasswordLen = 8
	maxPasswordLen = 72 // bcrypt's input limit
)

var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

func normalizeEmail(email string) string { return strings.TrimSpace(email) }

// ValidEmail reports whether email has a plausible address shape.
func ValidEmail(email string) bool {
	return emailRe.MatchString(normalizeEmail(email))
}

func validateCredentials(email, password string) error {
	if !emailRe.MatchString(email) {
		return ValidationError{"a valid email address is required"}
	}
	if n := len(password); n < minPasswordLen || n > maxPasswordLen {
		return ValidationError{fmt.Sprintf(
			"password must be between %d and %d characters", minPasswordLen, maxPasswordLen)}
	}
	return nil
}

// Signup creates a platform account and an initial session in one
// transaction. It returns the user and the raw session token, or ErrEmailTaken
// / ValidationError.
func Signup(ctx context.Context, pool *pgxpool.Pool, ttl time.Duration,
	email, password, name string) (User, string, error) {

	email = normalizeEmail(email)
	if err := validateCredentials(email, password); err != nil {
		return User{}, "", err
	}
	if strings.TrimSpace(name) == "" {
		return User{}, "", ValidationError{"name is required"}
	}
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, "", fmt.Errorf("hashing password: %w", err)
	}

	var (
		user  User
		token string
	)
	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		user, err = CreateUser(ctx, tx, email, hash, strings.TrimSpace(name))
		if err != nil {
			return err
		}
		token, err = IssueSession(ctx, tx, user.ID, ttl)
		return err
	})
	if err != nil {
		return User{}, "", err
	}
	return user, token, nil
}

// Login verifies credentials and issues a session. It returns
// ErrInvalidCredentials for both an unknown email and a wrong password.
func Login(ctx context.Context, pool *pgxpool.Pool, ttl time.Duration,
	email, password string) (User, string, error) {

	email = normalizeEmail(email)
	user, hash, err := getCredentials(ctx, pool, email)
	if errors.Is(err, ErrUserNotFound) {
		return User{}, "", ErrInvalidCredentials
	}
	if err != nil {
		return User{}, "", err
	}
	if !VerifyPassword(hash, password) {
		return User{}, "", ErrInvalidCredentials
	}
	token, err := IssueSession(ctx, pool, user.ID, ttl)
	if err != nil {
		return User{}, "", err
	}
	return user, token, nil
}

// Logout revokes the session identified by the raw token.
func Logout(ctx context.Context, pool *pgxpool.Pool, token string) error {
	return RevokeSession(ctx, pool, token)
}

// CreateAccount validates and inserts a new platform account using q (which
// may be a transaction). Unlike Signup it issues no session and takes the
// email as-is — used by the invitation-acceptance flow, where the email comes
// from the invitation itself.
func CreateAccount(ctx context.Context, q db.Querier, email, password, name string) (User, error) {
	if n := len(password); n < minPasswordLen || n > maxPasswordLen {
		return User{}, ValidationError{fmt.Sprintf(
			"password must be between %d and %d characters", minPasswordLen, maxPasswordLen)}
	}
	if strings.TrimSpace(name) == "" {
		return User{}, ValidationError{"name is required"}
	}
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, fmt.Errorf("hashing password: %w", err)
	}
	return CreateUser(ctx, q, normalizeEmail(email), hash, strings.TrimSpace(name))
}

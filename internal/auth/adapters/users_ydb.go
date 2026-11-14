package adapters

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/db"
)

// YDBUsers is the YDB-backed implementation of domain.UserRepository, for the
// serverless runtime. Emails match case-insensitively through the lower-cased
// email_key column, as the citext column does on Postgres.
type YDBUsers struct {
	driver *ydb.Driver
}

var _ domain.UserRepository = (*YDBUsers)(nil)

// NewYDBUsers builds a YDBUsers repository over the driver.
func NewYDBUsers(d *ydb.Driver) *YDBUsers {
	return &YDBUsers{driver: d}
}

// ydbUserColumns is the projection scanUser reads, in order.
const ydbUserColumns = `id, email, name, COALESCE(locale, ""u) AS locale, email_verified_at`

// Create inserts a new user with an already-hashed password and returns the
// persisted user. It returns domain.ErrEmailTaken when the email is already
// registered.
func (r *YDBUsers) Create(ctx context.Context, u *domain.User, passwordHash string) (*domain.User, error) {
	id := uuid.NewString()
	if err := insertYDBUser(ctx, r.driver.Query(), id, u, passwordHash); err != nil {
		return nil, err
	}
	return newlyCreatedUser(id, u), nil
}

// CreateWithSession atomically inserts a new user and issues an initial
// session. The SDK may re-run the transaction, so issueSession may run more
// than once. The id is fixed before the closure and the result is built after
// it, so nothing a retry repeats leaks out.
func (r *YDBUsers) CreateWithSession(ctx context.Context, u *domain.User, passwordHash string,
	issueSession func(userID string) (*domain.Session, string, error)) (*domain.User, error) {

	id := uuid.NewString()
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		if err := insertYDBUser(ctx, tx, id, u, passwordHash); err != nil {
			return err
		}
		session, tokenHash, err := issueSession(id)
		if err != nil {
			return err
		}
		return insertYDBSession(ctx, tx, session, tokenHash)
	})
	if err != nil {
		return nil, err
	}
	return newlyCreatedUser(id, u), nil
}

// CreateWithVerification atomically inserts a new user and its first
// email-verification challenge. It has the same retry contract as
// CreateWithSession.
func (r *YDBUsers) CreateWithVerification(ctx context.Context, u *domain.User, passwordHash string,
	issueVerification func(userID string) (*domain.EmailVerification, string, error)) (*domain.User, error) {

	id := uuid.NewString()
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		if err := insertYDBUser(ctx, tx, id, u, passwordHash); err != nil {
			return err
		}
		verification, tokenHash, err := issueVerification(id)
		if err != nil {
			return err
		}
		_, err = insertYDBVerificationToken(ctx, tx, verification, tokenHash)
		return err
	})
	if err != nil {
		return nil, err
	}
	return newlyCreatedUser(id, u), nil
}

// insertYDBUser inserts one platform_users row through q, which may be the
// query client or a transaction. A conflict on the unique email_key index maps
// to domain.ErrEmailTaken. YDB reports a primary-key collision the same way,
// which a random UUID makes negligible.
func insertYDBUser(ctx context.Context, q query.Executor, id string, u *domain.User, passwordHash string) error {
	err := q.Exec(ctx, `DECLARE $id AS Utf8;
DECLARE $email AS Utf8;
DECLARE $email_key AS Utf8;
DECLARE $password_hash AS Utf8;
DECLARE $name AS Utf8;
INSERT INTO platform_users (id, email, email_key, password_hash, name, created_at, updated_at)
VALUES ($id, $email, $email_key, $password_hash, $name, CurrentUtcTimestamp(), CurrentUtcTimestamp());`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$id").Text(id).
			Param("$email").Text(u.Email().String()).
			Param("$email_key").Text(ydbEmailKey(u.Email().String())).
			Param("$password_hash").Text(passwordHash).
			Param("$name").Text(u.Name()).
			Build()))
	if db.IsYDBConflict(err) {
		return domain.ErrEmailTaken
	}
	if err != nil {
		return fmt.Errorf("inserting platform user: %w", err)
	}
	return nil
}

// newlyCreatedUser is the persisted form of a just-inserted user, which has no
// locale yet and is unverified, as on Postgres.
func newlyCreatedUser(id string, u *domain.User) *domain.User {
	return domain.HydrateUser(id, u.Email().String(), u.Name(), "", nil)
}

// MarkEmailVerified records that the user verified their email address at now.
// It is idempotent: the UPDATE only touches a still-unverified row, so a repeat
// call leaves the original instant untouched.
func (r *YDBUsers) MarkEmailVerified(ctx context.Context, userID string, now time.Time) error {
	if err := r.driver.Query().Exec(ctx, `DECLARE $id AS Utf8;
DECLARE $email_verified_at AS Timestamp;
UPDATE platform_users SET email_verified_at = $email_verified_at, updated_at = CurrentUtcTimestamp()
WHERE id = $id AND email_verified_at IS NULL;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$id").Text(userID).
			Param("$email_verified_at").Timestamp(now).
			Build())); err != nil {
		return fmt.Errorf("marking email verified: %w", err)
	}
	return nil
}

// GetByID returns the user with the given id, or domain.ErrUserNotFound.
func (r *YDBUsers) GetByID(ctx context.Context, id string) (*domain.User, error) {
	row, err := r.driver.Query().QueryRow(ctx, `DECLARE $id AS Utf8;
SELECT `+ydbUserColumns+`
FROM platform_users
WHERE id = $id;`,
		query.WithParameters(ydb.ParamsBuilder().Param("$id").Text(id).Build()))
	return scanYDBUser(row, err, "loading platform user")
}

// UpdateLocale persists the user's interface-language preference. It returns
// domain.ErrUserNotFound when no user has the given id: UPDATE ... RETURNING
// yields no row for a missing id.
func (r *YDBUsers) UpdateLocale(ctx context.Context, userID string, locale domain.Locale) error {
	_, err := r.driver.Query().QueryRow(ctx, `DECLARE $id AS Utf8;
DECLARE $locale AS Utf8;
UPDATE platform_users SET locale = $locale, updated_at = CurrentUtcTimestamp()
WHERE id = $id
RETURNING id;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$id").Text(userID).
			Param("$locale").Text(locale.String()).
			Build()))
	if errors.Is(err, query.ErrNoRows) {
		return domain.ErrUserNotFound
	}
	if err != nil {
		return fmt.Errorf("updating user locale: %w", err)
	}
	return nil
}

// LookupByEmail returns the user with the given email, matched ignoring case,
// or domain.ErrUserNotFound.
func (r *YDBUsers) LookupByEmail(ctx context.Context, email string) (*domain.User, error) {
	row, err := r.driver.Query().QueryRow(ctx, `DECLARE $email_key AS Utf8;
SELECT `+ydbUserColumns+`
FROM platform_users VIEW idx_platform_users_email_key
WHERE email_key = $email_key;`, ydbEmailKeyParam(email))
	return scanYDBUser(row, err, "looking up user")
}

// GetCredentials returns the user and the stored bcrypt hash for an email,
// matched ignoring case, or domain.ErrUserNotFound. The hash never leaves the
// adapter/app boundary.
func (r *YDBUsers) GetCredentials(ctx context.Context, email string) (*domain.User, string, error) {
	row, err := r.driver.Query().QueryRow(ctx, `DECLARE $email_key AS Utf8;
SELECT `+ydbUserColumns+`, password_hash
FROM platform_users VIEW idx_platform_users_email_key
WHERE email_key = $email_key;`, ydbEmailKeyParam(email))
	if errors.Is(err, query.ErrNoRows) {
		return nil, "", domain.ErrUserNotFound
	}
	if err != nil {
		return nil, "", fmt.Errorf("loading credentials: %w", err)
	}
	var id, gotEmail, name, locale, hash string
	var emailVerifiedAt *time.Time
	if err := row.Scan(&id, &gotEmail, &name, &locale, &emailVerifiedAt, &hash); err != nil {
		return nil, "", fmt.Errorf("scanning credentials: %w", err)
	}
	return domain.HydrateUser(id, gotEmail, name, locale, emailVerifiedAt), hash, nil
}

// ydbEmailKey is the case-insensitive lookup key for an email address.
func ydbEmailKey(email string) string { return strings.ToLower(email) }

// ydbEmailKeyParam binds $email_key for an email lookup.
func ydbEmailKeyParam(email string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().Param("$email_key").Text(ydbEmailKey(email)).Build())
}

// scanYDBUser maps a single-row read in ydbUserColumns order to the user,
// translating "no row" into domain.ErrUserNotFound.
func scanYDBUser(row query.Row, err error, what string) (*domain.User, error) {
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	var id, email, name, locale string
	var emailVerifiedAt *time.Time
	if err := row.Scan(&id, &email, &name, &locale, &emailVerifiedAt); err != nil {
		return nil, fmt.Errorf("%s: scanning: %w", what, err)
	}
	return domain.HydrateUser(id, email, name, locale, emailVerifiedAt), nil
}

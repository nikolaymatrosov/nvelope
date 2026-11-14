package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

// YDBUsers is the YDB-backed implementation of domain.UserRepository, for the
// serverless runtime. YDB has no row-level security, so every statement
// filters by tenant_id explicitly; the primary key (tenant_id, id) makes that
// filter a key-prefix lookup.
type YDBUsers struct {
	driver *ydb.Driver
}

var _ domain.UserRepository = (*YDBUsers)(nil)

// NewYDBUsers builds a YDBUsers repository over the driver.
func NewYDBUsers(d *ydb.Driver) *YDBUsers {
	return &YDBUsers{driver: d}
}

// ydbUserColumns is the projection ydbUserFromRow reads, in order.
const ydbUserColumns = `id, tenant_id, platform_user_id, email, name, status,
    totp_enabled, totp_secret, created_at, updated_at`

const ydbSelectUser = `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT ` + ydbUserColumns + `
FROM users
WHERE tenant_id = $tenant_id AND id = $id;`

// Add persists a new tenant-plane user under a fresh UUID and returns the id.
// A second user for the same platform user or email in the tenant collides
// with a unique index and is returned as an error, as on Postgres.
func (r *YDBUsers) Add(ctx context.Context, tenantID string, u *domain.TenantUser) (string, error) {
	id := uuid.NewString()
	if err := r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $platform_user_id AS Utf8;
DECLARE $email AS Utf8;
DECLARE $name AS Utf8;
DECLARE $status AS Utf8;
DECLARE $totp_enabled AS Bool;
DECLARE $totp_secret AS Optional<String>;
INSERT INTO users
    (tenant_id, id, platform_user_id, email, name, status, totp_enabled, totp_secret,
     created_at, updated_at)
VALUES ($tenant_id, $id, $platform_user_id, $email, $name, $status, $totp_enabled, $totp_secret,
     CurrentUtcTimestamp(), CurrentUtcTimestamp());`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$id").Text(id).
			Param("$platform_user_id").Text(u.PlatformUserID()).
			Param("$email").Text(u.Email()).
			Param("$name").Text(u.Name()).
			Param("$status").Text(string(u.Status())).
			Param("$totp_enabled").Bool(u.TOTPEnabled()).
			Param("$totp_secret").BeginOptional().Bytes(ydbOptionalBytes(u.TOTPSecret())).EndOptional().
			Build())); err != nil {
		return "", fmt.Errorf("inserting user: %w", err)
	}
	return id, nil
}

// Update loads the user, runs fn, and persists the result in one serializable
// transaction. The SDK retries the transaction on conflict, so fn may run more
// than once and must be retry-safe.
func (r *YDBUsers) Update(ctx context.Context, tenantID, id string,
	fn func(*domain.TenantUser) (*domain.TenantUser, error)) error {

	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		loaded, err := ydbUserFromRow(tx.QueryRow(ctx, ydbSelectUser, ydbTenantAndID(tenantID, id)))
		if err != nil {
			return err
		}
		updated, err := fn(loaded)
		if err != nil {
			return err
		}
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $name AS Utf8;
DECLARE $status AS Utf8;
DECLARE $totp_enabled AS Bool;
DECLARE $totp_secret AS Optional<String>;
UPDATE users SET
    name = $name, status = $status, totp_enabled = $totp_enabled, totp_secret = $totp_secret,
    updated_at = CurrentUtcTimestamp()
WHERE tenant_id = $tenant_id AND id = $id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$id").Text(id).
				Param("$name").Text(updated.Name()).
				Param("$status").Text(string(updated.Status())).
				Param("$totp_enabled").Bool(updated.TOTPEnabled()).
				Param("$totp_secret").BeginOptional().Bytes(ydbOptionalBytes(updated.TOTPSecret())).EndOptional().
				Build())); err != nil {
			return fmt.Errorf("updating user: %w", err)
		}
		return nil
	})
}

// Get returns the user, or domain.ErrUserNotFound.
func (r *YDBUsers) Get(ctx context.Context, tenantID, id string) (*domain.TenantUser, error) {
	return ydbUserFromRow(r.driver.Query().QueryRow(ctx, ydbSelectUser, ydbTenantAndID(tenantID, id)))
}

// ByPlatformUser returns the tenant-plane user for a control-plane identity,
// or domain.ErrUserNotFound.
func (r *YDBUsers) ByPlatformUser(ctx context.Context, tenantID, platformUserID string) (*domain.TenantUser, error) {
	return ydbUserFromRow(r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $platform_user_id AS Utf8;
SELECT `+ydbUserColumns+`
FROM users VIEW idx_users_platform_user
WHERE tenant_id = $tenant_id AND platform_user_id = $platform_user_id;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$platform_user_id").Text(platformUserID).
			Build())))
}

// ydbUserFromRow maps a single-row read in ydbUserColumns order to the user,
// translating "no row" into domain.ErrUserNotFound.
func ydbUserFromRow(row query.Row, err error) (*domain.TenantUser, error) {
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading user: %w", err)
	}
	var id, tenantID, platformUserID, email, name, status string
	var totpEnabled bool
	var totpSecret *[]byte
	var createdAt, updatedAt time.Time
	if err := row.Scan(&id, &tenantID, &platformUserID, &email, &name, &status,
		&totpEnabled, &totpSecret, &createdAt, &updatedAt); err != nil {
		return nil, fmt.Errorf("scanning user: %w", err)
	}
	var secret []byte
	if totpSecret != nil {
		secret = *totpSecret
	}
	return domain.HydrateTenantUser(id, tenantID, platformUserID, email, name,
		domain.UserStatus(status), totpEnabled, secret, createdAt, updatedAt), nil
}

// ydbOptionalBytes is b as an Optional<String> parameter: NULL when b is nil,
// as pgx writes a nil []byte.
func ydbOptionalBytes(b []byte) *[]byte {
	if b == nil {
		return nil
	}
	return &b
}

// ydbTenantParam binds the $tenant_id parameter.
func ydbTenantParam(tenantID string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().Param("$tenant_id").Text(tenantID).Build())
}

// ydbTenantAndID binds the (tenant_id, id) primary key.
func ydbTenantAndID(tenantID, id string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$id").Text(id).
		Build())
}

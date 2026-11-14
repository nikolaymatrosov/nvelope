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

// YDBAPIKeys is the YDB-backed implementation of domain.APIKeyRepository, for
// the serverless runtime. Only the hash of a key is stored, and its
// permissions are a JSON array in a Utf8 column, as for roles.
type YDBAPIKeys struct {
	driver *ydb.Driver
}

var _ domain.APIKeyRepository = (*YDBAPIKeys)(nil)

// NewYDBAPIKeys builds a YDBAPIKeys repository over the driver.
func NewYDBAPIKeys(d *ydb.Driver) *YDBAPIKeys {
	return &YDBAPIKeys{driver: d}
}

// ydbAPIKeyColumns is the projection scanYDBAPIKey reads, in order.
const ydbAPIKeyColumns = `id, tenant_id, name, token_hash, permissions, created_by,
    created_at, last_used_at, revoked_at`

// Add persists a new API key under a fresh UUID and returns the id.
func (r *YDBAPIKeys) Add(ctx context.Context, tenantID string, k *domain.APIKey) (string, error) {
	id := uuid.NewString()
	if err := r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $name AS Utf8;
DECLARE $token_hash AS Utf8;
DECLARE $permissions AS Utf8;
DECLARE $created_by AS Utf8;
INSERT INTO api_keys (tenant_id, id, name, token_hash, permissions, created_by, created_at)
VALUES ($tenant_id, $id, $name, $token_hash, $permissions, $created_by, CurrentUtcTimestamp());`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$id").Text(id).
			Param("$name").Text(k.Name()).
			Param("$token_hash").Text(k.TokenHash()).
			Param("$permissions").Text(ydbPermissions(k.Permissions())).
			Param("$created_by").Text(k.CreatedBy()).
			Build())); err != nil {
		return "", fmt.Errorf("inserting API key: %w", err)
	}
	return id, nil
}

// ByTokenHash returns the tenant's API key for a token hash, revoked or not,
// or domain.ErrAPIKeyNotFound.
func (r *YDBAPIKeys) ByTokenHash(ctx context.Context, tenantID, tokenHash string) (*domain.APIKey, error) {
	row, err := r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $token_hash AS Utf8;
SELECT `+ydbAPIKeyColumns+`
FROM api_keys VIEW idx_api_keys_token_hash
WHERE token_hash = $token_hash AND tenant_id = $tenant_id;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$token_hash").Text(tokenHash).
			Build()))
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrAPIKeyNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading API key: %w", err)
	}
	return scanYDBAPIKey(row)
}

// Revoke marks the API key revoked. It returns domain.ErrAPIKeyNotFound when
// the tenant has no such key or the key is already revoked: UPDATE ...
// RETURNING yields no row then.
func (r *YDBAPIKeys) Revoke(ctx context.Context, tenantID, id string) error {
	_, err := r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
UPDATE api_keys SET revoked_at = CurrentUtcTimestamp()
WHERE tenant_id = $tenant_id AND id = $id AND revoked_at IS NULL
RETURNING id;`, ydbTenantAndID(tenantID, id))
	if errors.Is(err, query.ErrNoRows) {
		return domain.ErrAPIKeyNotFound
	}
	if err != nil {
		return fmt.Errorf("revoking API key: %w", err)
	}
	return nil
}

// TouchLastUsed records that the key was just used to authenticate. Touching
// an unknown key is a no-op.
func (r *YDBAPIKeys) TouchLastUsed(ctx context.Context, tenantID, id string) error {
	if err := r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
UPDATE api_keys SET last_used_at = CurrentUtcTimestamp()
WHERE tenant_id = $tenant_id AND id = $id;`, ydbTenantAndID(tenantID, id)); err != nil {
		return fmt.Errorf("touching API key: %w", err)
	}
	return nil
}

// All returns every API key in the tenant, newest first.
func (r *YDBAPIKeys) All(ctx context.Context, tenantID string) ([]*domain.APIKey, error) {
	rs, err := r.driver.Query().QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
SELECT `+ydbAPIKeyColumns+`
FROM api_keys
WHERE tenant_id = $tenant_id
ORDER BY created_at DESC;`, ydbTenantParam(tenantID))
	if err != nil {
		return nil, fmt.Errorf("listing API keys: %w", err)
	}
	defer func() { _ = rs.Close(ctx) }()
	var keys []*domain.APIKey
	for row, err := range rs.Rows(ctx) {
		if err != nil {
			return nil, fmt.Errorf("listing API keys: %w", err)
		}
		k, err := scanYDBAPIKey(row)
		if err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, nil
}

// scanYDBAPIKey reads one row in ydbAPIKeyColumns order.
func scanYDBAPIKey(row query.Row) (*domain.APIKey, error) {
	var id, tenantID, name, tokenHash, raw, createdBy string
	var createdAt time.Time
	var lastUsedAt, revokedAt *time.Time
	if err := row.Scan(&id, &tenantID, &name, &tokenHash, &raw, &createdBy,
		&createdAt, &lastUsedAt, &revokedAt); err != nil {
		return nil, fmt.Errorf("scanning API key: %w", err)
	}
	perms, err := ydbPermissionsFrom(raw)
	if err != nil {
		return nil, err
	}
	return domain.HydrateAPIKey(id, tenantID, name, tokenHash, perms,
		createdBy, createdAt, lastUsedAt, revokedAt), nil
}

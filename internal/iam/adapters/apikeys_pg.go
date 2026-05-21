package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/db"
	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// APIKeys is the pgx-backed implementation of domain.APIKeyRepository.
type APIKeys struct {
	pool *pgxpool.Pool
}

var _ domain.APIKeyRepository = (*APIKeys)(nil)

// NewAPIKeys builds an APIKeys repository over the given pool.
func NewAPIKeys(pool *pgxpool.Pool) *APIKeys {
	return &APIKeys{pool: pool}
}

const apiKeyColumns = "id, tenant_id, name, token_hash, permissions, created_by, " +
	"created_at, last_used_at, revoked_at"

func scanAPIKey(row pgx.Row) (*domain.APIKey, error) {
	var id, tenantID, name, tokenHash, createdBy string
	var perms []string
	var createdAt time.Time
	var lastUsedAt, revokedAt *time.Time
	if err := row.Scan(&id, &tenantID, &name, &tokenHash, &perms, &createdBy,
		&createdAt, &lastUsedAt, &revokedAt); err != nil {
		return nil, err
	}
	return domain.HydrateAPIKey(id, tenantID, name, tokenHash, permsFromStrings(perms),
		createdBy, createdAt, lastUsedAt, revokedAt), nil
}

// Add persists a new API key and returns its database-assigned id.
func (r *APIKeys) Add(ctx context.Context, tenantID string, k *domain.APIKey) (string, error) {
	var id string
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`INSERT INTO api_keys (tenant_id, name, token_hash, permissions, created_by)
			 VALUES (@tenant_id, @name, @token_hash, @permissions, @created_by)
			 RETURNING id`,
			pgx.NamedArgs{
				"tenant_id":   tenantID,
				"name":        k.Name(),
				"token_hash":  k.TokenHash(),
				"permissions": permStrings(k.Permissions()),
				"created_by":  k.CreatedBy(),
			}).Scan(&id)
	})
	if err != nil {
		return "", fmt.Errorf("inserting API key: %w", err)
	}
	return id, nil
}

// ByTokenHash returns the API key for a token hash, or domain.ErrAPIKeyNotFound.
func (r *APIKeys) ByTokenHash(ctx context.Context, tenantID, tokenHash string) (*domain.APIKey, error) {
	var out *domain.APIKey
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		k, err := scanAPIKey(tx.QueryRow(ctx,
			"SELECT "+apiKeyColumns+" FROM api_keys WHERE token_hash = $1", tokenHash))
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrAPIKeyNotFound
		}
		if err != nil {
			return fmt.Errorf("loading API key: %w", err)
		}
		out = k
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Revoke marks the API key revoked.
func (r *APIKeys) Revoke(ctx context.Context, tenantID, id string) error {
	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			"UPDATE api_keys SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL", id)
		if db.IsInvalidInput(err) {
			return domain.ErrAPIKeyNotFound
		}
		if err != nil {
			return fmt.Errorf("revoking API key: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrAPIKeyNotFound
		}
		return nil
	})
}

// TouchLastUsed records that the key was just used to authenticate.
func (r *APIKeys) TouchLastUsed(ctx context.Context, tenantID, id string) error {
	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "UPDATE api_keys SET last_used_at = now() WHERE id = $1", id)
		if err != nil {
			return fmt.Errorf("touching API key: %w", err)
		}
		return nil
	})
}

// All returns every API key in the tenant, newest first.
func (r *APIKeys) All(ctx context.Context, tenantID string) ([]*domain.APIKey, error) {
	var out []*domain.APIKey
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			"SELECT "+apiKeyColumns+" FROM api_keys ORDER BY created_at DESC")
		if err != nil {
			return fmt.Errorf("listing API keys: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			k, err := scanAPIKey(rows)
			if err != nil {
				return err
			}
			out = append(out, k)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

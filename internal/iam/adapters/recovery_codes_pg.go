package adapters

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// RecoveryCodes is the pgx-backed implementation of
// domain.RecoveryCodeRepository.
type RecoveryCodes struct {
	pool *pgxpool.Pool
}

var _ domain.RecoveryCodeRepository = (*RecoveryCodes)(nil)

// NewRecoveryCodes builds a RecoveryCodes repository over the given pool.
func NewRecoveryCodes(pool *pgxpool.Pool) *RecoveryCodes {
	return &RecoveryCodes{pool: pool}
}

// AddBatch replaces a user's recovery codes with a fresh batch.
func (r *RecoveryCodes) AddBatch(ctx context.Context, tenantID, userID string,
	codeHashes []string) error {

	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			"DELETE FROM recovery_codes WHERE user_id = $1", userID); err != nil {
			return fmt.Errorf("clearing recovery codes: %w", err)
		}
		for _, hash := range codeHashes {
			if _, err := tx.Exec(ctx,
				`INSERT INTO recovery_codes (tenant_id, user_id, code_hash)
				 VALUES ($1, $2, $3)`,
				tenantID, userID, hash); err != nil {
				return fmt.Errorf("inserting recovery code: %w", err)
			}
		}
		return nil
	})
}

// Consume marks a matching unused recovery code used.
func (r *RecoveryCodes) Consume(ctx context.Context, tenantID, userID, codeHash string) (bool, error) {
	var consumed bool
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`UPDATE recovery_codes SET used_at = now()
			 WHERE user_id = $1 AND code_hash = $2 AND used_at IS NULL`,
			userID, codeHash)
		if err != nil {
			return fmt.Errorf("consuming recovery code: %w", err)
		}
		consumed = tag.RowsAffected() > 0
		return nil
	})
	if err != nil {
		return false, err
	}
	return consumed, nil
}

// DeleteForUser removes every recovery code a user holds.
func (r *RecoveryCodes) DeleteForUser(ctx context.Context, tenantID, userID string) error {
	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			"DELETE FROM recovery_codes WHERE user_id = $1", userID); err != nil {
			return fmt.Errorf("deleting recovery codes: %w", err)
		}
		return nil
	})
}

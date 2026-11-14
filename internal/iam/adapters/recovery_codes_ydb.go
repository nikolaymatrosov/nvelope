package adapters

import (
	"context"
	"errors"
	"fmt"

	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

// YDBRecoveryCodes is the YDB-backed implementation of
// domain.RecoveryCodeRepository, for the serverless runtime. A code is keyed
// (tenant_id, user_id, code_hash), so a user's codes are one key-prefix range.
type YDBRecoveryCodes struct {
	driver *ydb.Driver
}

var _ domain.RecoveryCodeRepository = (*YDBRecoveryCodes)(nil)

// NewYDBRecoveryCodes builds a YDBRecoveryCodes repository over the driver.
func NewYDBRecoveryCodes(d *ydb.Driver) *YDBRecoveryCodes {
	return &YDBRecoveryCodes{driver: d}
}

const ydbDeleteRecoveryCodes = `DECLARE $tenant_id AS Utf8;
DECLARE $user_id AS Utf8;
DELETE FROM recovery_codes
WHERE tenant_id = $tenant_id AND user_id = $user_id;`

// AddBatch replaces a user's recovery codes with a fresh batch in one
// transaction. A retried transaction deletes and inserts the same rows again.
func (r *YDBRecoveryCodes) AddBatch(ctx context.Context, tenantID, userID string, codeHashes []string) error {
	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		if err := tx.Exec(ctx, ydbDeleteRecoveryCodes, ydbTenantAndUser(tenantID, userID)); err != nil {
			return fmt.Errorf("clearing recovery codes: %w", err)
		}
		// One statement per code: a batch is ten codes (recoveryCodeCount in
		// internal/iam/app/command).
		for _, hash := range codeHashes {
			if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $user_id AS Utf8;
DECLARE $code_hash AS Utf8;
INSERT INTO recovery_codes (tenant_id, user_id, code_hash)
VALUES ($tenant_id, $user_id, $code_hash);`,
				query.WithParameters(ydb.ParamsBuilder().
					Param("$tenant_id").Text(tenantID).
					Param("$user_id").Text(userID).
					Param("$code_hash").Text(hash).
					Build())); err != nil {
				return fmt.Errorf("inserting recovery code: %w", err)
			}
		}
		return nil
	})
}

// Consume marks a matching unused recovery code used, reporting whether one
// existed. UPDATE ... RETURNING yields a row only when it marked a code.
func (r *YDBRecoveryCodes) Consume(ctx context.Context, tenantID, userID, codeHash string) (bool, error) {
	_, err := r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $user_id AS Utf8;
DECLARE $code_hash AS Utf8;
UPDATE recovery_codes SET used_at = CurrentUtcTimestamp()
WHERE tenant_id = $tenant_id AND user_id = $user_id AND code_hash = $code_hash AND used_at IS NULL
RETURNING code_hash;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$user_id").Text(userID).
			Param("$code_hash").Text(codeHash).
			Build()))
	if errors.Is(err, query.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("consuming recovery code: %w", err)
	}
	return true, nil
}

// DeleteForUser removes every recovery code a user holds.
func (r *YDBRecoveryCodes) DeleteForUser(ctx context.Context, tenantID, userID string) error {
	if err := r.driver.Query().Exec(ctx, ydbDeleteRecoveryCodes, ydbTenantAndUser(tenantID, userID)); err != nil {
		return fmt.Errorf("deleting recovery codes: %w", err)
	}
	return nil
}

// ydbTenantAndUser binds the (tenant_id, user_id) key prefix.
func ydbTenantAndUser(tenantID, userID string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$user_id").Text(userID).
		Build())
}

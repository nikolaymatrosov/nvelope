package adapters

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// withTenant runs fn inside a transaction bound to tenantID. It opens a
// transaction, sets the transaction-local app.tenant_id GUC, invokes fn, then
// commits when fn returns nil or rolls back otherwise.
//
// Every tenant-plane (RLS-protected) read or write MUST go through this
// helper: outside a tenant-bound transaction the GUC is unset and the RLS
// policies expose zero rows — isolation fails closed. The helper is
// adapter-private; the application layer never sees the transaction or the GUC.
func withTenant(ctx context.Context, pool *pgxpool.Pool, tenantID string,
	fn func(ctx context.Context, tx pgx.Tx) error) error {

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning tenant transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// set_config with a bound parameter and is_local=true scopes the value to
	// this transaction, so it cannot leak across pooled connections. SET LOCAL
	// cannot take a bound parameter, so set_config is used instead.
	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID); err != nil {
		return fmt.Errorf("binding tenant: %w", err)
	}

	if err := fn(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing tenant transaction: %w", err)
	}
	return nil
}

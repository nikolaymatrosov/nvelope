// Package tenantdb provides the Row-Level-Security-bound transaction helper
// shared by every tenant-plane bounded context (tenant, iam, audience). It is
// the one implementation of the isolation primitive the constitution mandates:
// every tenant-plane read or write runs inside a transaction that binds
// app.tenant_id, and outside such a transaction the RLS policies expose zero
// rows — isolation fails closed.
package tenantdb

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WithTenant runs fn inside a transaction bound to tenantID. It opens a
// transaction, sets the transaction-local app.tenant_id GUC, invokes fn, then
// commits when fn returns nil or rolls back otherwise.
//
// Every tenant-plane (RLS-protected) read or write MUST go through this
// helper: outside a tenant-bound transaction the GUC is unset and the RLS
// policies expose zero rows — isolation fails closed. The transaction is owned
// by the adapter layer; the application layer never sees the transaction or
// the GUC.
func WithTenant(ctx context.Context, pool *pgxpool.Pool, tenantID string,
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

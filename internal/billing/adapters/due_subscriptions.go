package adapters

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/billing/domain"
)

// DueSubscriptions resolves the subscriptions due for a charge through the
// billing_due_subscriptions() SECURITY DEFINER function. The function projects
// only (tenant_id, subscription_id, reason), so the cross-tenant read never
// exposes full subscription rows and RLS stays the authoritative backstop.
type DueSubscriptions struct {
	pool *pgxpool.Pool
}

var _ domain.DueSubscriptionReader = (*DueSubscriptions)(nil)

// NewDueSubscriptions builds a DueSubscriptions reader over the given pool.
func NewDueSubscriptions(pool *pgxpool.Pool) *DueSubscriptions {
	return &DueSubscriptions{pool: pool}
}

// ListDue returns every subscription due for a renewal or a dunning retry.
func (r *DueSubscriptions) ListDue(ctx context.Context) ([]domain.DueSubscription, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT tenant_id, subscription_id, reason FROM billing_due_subscriptions()`)
	if err != nil {
		return nil, fmt.Errorf("listing due subscriptions: %w", err)
	}
	defer rows.Close()

	var out []domain.DueSubscription
	for rows.Next() {
		var d domain.DueSubscription
		if err := rows.Scan(&d.TenantID, &d.SubscriptionID, &d.Reason); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

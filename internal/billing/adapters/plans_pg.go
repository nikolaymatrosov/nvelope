// Package adapters holds the billing context's driven and driving adapters:
// the pgx-backed repositories, the deterministic payment gateway, the River
// workers, and the quota gate.
package adapters

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/billing/domain"
)

// Plans is the pgx-backed implementation of domain.PlanRepository. The plans
// catalog is control-plane data with no RLS, so its reads go through the pool.
type Plans struct {
	pool *pgxpool.Pool
}

var _ domain.PlanRepository = (*Plans)(nil)

// NewPlans builds a Plans repository over the given pool.
func NewPlans(pool *pgxpool.Pool) *Plans {
	return &Plans{pool: pool}
}

const planColumns = `id, code, name, price_minor, currency, billing_period,
	included_sends, overage_mode, overage_price_minor, status`

// ListPublished returns every subscribable plan, cheapest first.
func (r *Plans) ListPublished(ctx context.Context) ([]*domain.Plan, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+planColumns+` FROM plans WHERE status = 'published' ORDER BY price_minor`)
	if err != nil {
		return nil, fmt.Errorf("listing published plans: %w", err)
	}
	defer rows.Close()

	var out []*domain.Plan
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Get returns one plan by id, or domain.ErrPlanNotFound.
func (r *Plans) Get(ctx context.Context, id string) (*domain.Plan, error) {
	p, err := scanPlan(r.pool.QueryRow(ctx, `SELECT `+planColumns+` FROM plans WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrPlanNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading plan: %w", err)
	}
	return p, nil
}

// scanPlan reads one plan row in planColumns order.
func scanPlan(row pgx.Row) (*domain.Plan, error) {
	var id, code, name, currency, overageMode, status string
	var priceMinor, includedSends, overagePriceMinor int64
	var period pgtype.Interval
	if err := row.Scan(&id, &code, &name, &priceMinor, &currency, &period,
		&includedSends, &overageMode, &overagePriceMinor, &status); err != nil {
		return nil, err
	}
	days := int(period.Days) + int(period.Microseconds/86_400_000_000)
	return domain.HydratePlan(id, code, name,
		domain.NewMoney(priceMinor, currency),
		domain.NewBillingPeriod(int(period.Months), days),
		includedSends, domain.OverageMode(overageMode),
		domain.NewMoney(overagePriceMinor, currency),
		domain.PlanStatus(status)), nil
}

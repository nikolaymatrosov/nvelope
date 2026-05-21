package adapters

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/billing/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// Usage is the pgx-backed implementation of domain.UsageRepository, covering
// raw usage events and the period counters rolled up from them.
type Usage struct {
	pool *pgxpool.Pool
}

var _ domain.UsageRepository = (*Usage)(nil)

// NewUsage builds a Usage repository over the given pool.
func NewUsage(pool *pgxpool.Pool) *Usage {
	return &Usage{pool: pool}
}

// RecordEvents inserts usage events, treating a repeated (tenant, type,
// source_ref) as a no-op so a retried send never double-counts.
func (r *Usage) RecordEvents(ctx context.Context, tenantID string, events []*domain.UsageEvent) error {
	if len(events) == 0 {
		return nil
	}
	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		for _, e := range events {
			_, err := tx.Exec(ctx,
				`INSERT INTO usage_events
				    (tenant_id, event_type, quantity, source_ref, occurred_at, period_start)
				 VALUES (@tenant_id, @event_type, @quantity, @source_ref, @occurred_at, @period_start)
				 ON CONFLICT (tenant_id, event_type, source_ref) DO NOTHING`,
				pgx.NamedArgs{
					"tenant_id":    tenantID,
					"event_type":   string(e.EventType()),
					"quantity":     e.Quantity(),
					"source_ref":   e.SourceRef(),
					"occurred_at":  e.OccurredAt(),
					"period_start": e.PeriodStart(),
				})
			if err != nil {
				return fmt.Errorf("recording usage event: %w", err)
			}
		}
		return nil
	})
}

// usageGroup is one (period_start, event_type) aggregate of un-rolled events.
type usageGroup struct {
	periodStart time.Time
	eventType   string
	sum         int64
}

// Rollup aggregates the tenant's not-yet-rolled events into period counters and
// stamps the processed events, all inside one transaction.
func (r *Usage) Rollup(ctx context.Context, tenantID string, allowance int64,
	period domain.BillingPeriod) error {

	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT period_start, event_type, sum(quantity)
			 FROM usage_events WHERE rolled_up_at IS NULL
			 GROUP BY period_start, event_type`)
		if err != nil {
			return fmt.Errorf("aggregating un-rolled usage events: %w", err)
		}
		var groups []usageGroup
		for rows.Next() {
			var g usageGroup
			if err := rows.Scan(&g.periodStart, &g.eventType, &g.sum); err != nil {
				rows.Close()
				return err
			}
			groups = append(groups, g)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		for _, g := range groups {
			periodEnd := period.AdvanceFrom(g.periodStart)
			var newTotal int64
			err := tx.QueryRow(ctx,
				`INSERT INTO usage_counters
				    (tenant_id, period_start, period_end, event_type, total_quantity)
				 VALUES (@tenant_id, @period_start, @period_end, @event_type, @total_quantity)
				 ON CONFLICT (tenant_id, period_start, event_type)
				 DO UPDATE SET total_quantity = usage_counters.total_quantity + EXCLUDED.total_quantity,
				               updated_at = now()
				 RETURNING total_quantity`,
				pgx.NamedArgs{
					"tenant_id":      tenantID,
					"period_start":   g.periodStart,
					"period_end":     periodEnd,
					"event_type":     g.eventType,
					"total_quantity": g.sum,
				}).Scan(&newTotal)
			if err != nil {
				return fmt.Errorf("upserting usage counter: %w", err)
			}
			included, overage := domain.SplitUsage(newTotal, allowance)
			if _, err := tx.Exec(ctx,
				`UPDATE usage_counters
				    SET included_quantity = @included_quantity, overage_quantity = @overage_quantity
				 WHERE tenant_id = @tenant_id
				   AND period_start = @period_start
				   AND event_type = @event_type`,
				pgx.NamedArgs{
					"included_quantity": included,
					"overage_quantity":  overage,
					"tenant_id":         tenantID,
					"period_start":      g.periodStart,
					"event_type":        g.eventType,
				}); err != nil {
				return fmt.Errorf("recomputing usage counter split: %w", err)
			}
		}

		if _, err := tx.Exec(ctx,
			`UPDATE usage_events SET rolled_up_at = now() WHERE rolled_up_at IS NULL`); err != nil {
			return fmt.Errorf("stamping rolled-up usage events: %w", err)
		}
		return nil
	})
}

// CurrentUsage returns the metered send count for a period — the rolled-up
// counter total plus the not-yet-rolled events tail.
func (r *Usage) CurrentUsage(ctx context.Context, tenantID string, periodStart time.Time) (int64, error) {
	var used int64
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT
			    coalesce((SELECT sum(total_quantity) FROM usage_counters
			              WHERE period_start = $1), 0)
			  + coalesce((SELECT sum(quantity) FROM usage_events
			              WHERE period_start = $1 AND rolled_up_at IS NULL), 0)`,
			periodStart).Scan(&used)
	})
	if err != nil {
		return 0, fmt.Errorf("reading current usage: %w", err)
	}
	return used, nil
}

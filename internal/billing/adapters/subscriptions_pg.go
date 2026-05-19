package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/billing/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// Subscriptions is the pgx-backed implementation of
// domain.SubscriptionRepository. Every operation runs inside an RLS-bound
// tenant transaction.
type Subscriptions struct {
	pool *pgxpool.Pool
}

var _ domain.SubscriptionRepository = (*Subscriptions)(nil)

// NewSubscriptions builds a Subscriptions repository over the given pool.
func NewSubscriptions(pool *pgxpool.Pool) *Subscriptions {
	return &Subscriptions{pool: pool}
}

const subscriptionColumns = `id, tenant_id, plan_id, state, current_period_start,
	current_period_end, cancel_at_period_end, canceled_at`

// Add inserts a new subscription and returns its id. A clash with the partial
// unique index — a tenant already holding a non-canceled subscription — is
// surfaced as domain.ErrSubscriptionExists.
func (r *Subscriptions) Add(ctx context.Context, s *domain.Subscription) (string, error) {
	var id string
	err := tenantdb.WithTenant(ctx, r.pool, s.TenantID(), func(ctx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`INSERT INTO tenant_subscriptions
			    (tenant_id, plan_id, state, current_period_start, current_period_end,
			     cancel_at_period_end)
			 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
			s.TenantID(), s.PlanID(), string(s.State()),
			s.CurrentPeriodStart(), s.CurrentPeriodEnd(), s.CancelAtPeriodEnd()).Scan(&id)
		if isUniqueViolation(err) {
			return domain.ErrSubscriptionExists
		}
		if err != nil {
			return fmt.Errorf("inserting subscription: %w", err)
		}
		return nil
	})
	return id, err
}

// Get returns one subscription by id, or domain.ErrNoSubscription.
func (r *Subscriptions) Get(ctx context.Context, tenantID, id string) (*domain.Subscription, error) {
	var out *domain.Subscription
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		s, err := scanSubscription(tx.QueryRow(ctx,
			`SELECT `+subscriptionColumns+` FROM tenant_subscriptions WHERE id = $1`, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNoSubscription
		}
		if err != nil {
			return fmt.Errorf("loading subscription: %w", err)
		}
		out = s
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Current returns the tenant's single non-canceled subscription.
func (r *Subscriptions) Current(ctx context.Context, tenantID string) (*domain.Subscription, bool, error) {
	var out *domain.Subscription
	found := false
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		s, err := scanSubscription(tx.QueryRow(ctx,
			`SELECT `+subscriptionColumns+` FROM tenant_subscriptions
			 WHERE state <> 'canceled' LIMIT 1`))
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("loading current subscription: %w", err)
		}
		out, found = s, true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return out, found, nil
}

// Update loads the subscription, runs fn, and persists the result.
func (r *Subscriptions) Update(ctx context.Context, tenantID, id string,
	fn func(*domain.Subscription) (*domain.Subscription, error)) error {

	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		loaded, err := scanSubscription(tx.QueryRow(ctx,
			`SELECT `+subscriptionColumns+` FROM tenant_subscriptions WHERE id = $1 FOR UPDATE`, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNoSubscription
		}
		if err != nil {
			return fmt.Errorf("loading subscription for update: %w", err)
		}
		updated, err := fn(loaded)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx,
			`UPDATE tenant_subscriptions
			 SET state = $1, current_period_start = $2, current_period_end = $3,
			     cancel_at_period_end = $4, canceled_at = $5, updated_at = now()
			 WHERE id = $6`,
			string(updated.State()), updated.CurrentPeriodStart(), updated.CurrentPeriodEnd(),
			updated.CancelAtPeriodEnd(), updated.CanceledAt(), id)
		if err != nil {
			return fmt.Errorf("updating subscription: %w", err)
		}
		return nil
	})
}

// scanSubscription reads one subscription row in subscriptionColumns order.
func scanSubscription(row pgx.Row) (*domain.Subscription, error) {
	var id, tenantID, planID, state string
	var periodStart, periodEnd time.Time
	var cancelAtPeriodEnd bool
	var canceledAt *time.Time
	if err := row.Scan(&id, &tenantID, &planID, &state, &periodStart, &periodEnd,
		&cancelAtPeriodEnd, &canceledAt); err != nil {
		return nil, err
	}
	return domain.HydrateSubscription(id, tenantID, planID, domain.SubscriptionState(state),
		periodStart, periodEnd, cancelAtPeriodEnd, canceledAt), nil
}

// isUniqueViolation reports whether err is a PostgreSQL unique-constraint
// violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

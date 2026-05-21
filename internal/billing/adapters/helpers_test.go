package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// seedTenant inserts a tenant row directly and returns its id.
func seedTenant(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO tenants (name, slug, status) VALUES ('Workspace', $1, 'active') RETURNING id`,
		"bill-"+dbtest.RandString()).Scan(&id))
	return id
}

// seedPlan inserts a plan with the given status and returns its id. The plan is
// a monthly RUB plan covering 50 000 sends in block mode.
func seedPlan(t *testing.T, pool *pgxpool.Pool, status string) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO plans
		    (code, name, price_minor, currency, billing_period, included_sends,
		     overage_mode, overage_price_minor, status)
		 VALUES ($1, 'Starter', 990000, 'RUB', '1 month'::interval, 50000, 'block', 0, $2)
		 RETURNING id`,
		"plan-"+dbtest.RandString(), status).Scan(&id))
	return id
}

// seedPlanWith inserts a plan with the given status, overage mode, and
// included-send allowance, and returns its id.
func seedPlanWith(t *testing.T, pool *pgxpool.Pool, status, mode string, included int64) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO plans
		    (code, name, price_minor, currency, billing_period, included_sends,
		     overage_mode, overage_price_minor, status)
		 VALUES (@code, 'Plan', 990000, 'RUB', '1 month'::interval, @included_sends,
		         @overage_mode, 100, @status)
		 RETURNING id`,
		pgx.NamedArgs{
			"code":           "plan-" + dbtest.RandString(),
			"included_sends": included,
			"overage_mode":   mode,
			"status":         status,
		}).Scan(&id))
	return id
}

// seedSubscriptionState inserts a subscription in the given state with a
// current billing period and returns its id.
func seedSubscriptionState(t *testing.T, pool *pgxpool.Pool, tenantID, planID, state string) string {
	t.Helper()
	start := time.Now().UTC().Truncate(time.Second)
	var id string
	require.NoError(t, tenantdb.WithTenant(context.Background(), pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx,
				`INSERT INTO tenant_subscriptions
				    (tenant_id, plan_id, state, current_period_start, current_period_end)
				 VALUES (@tenant_id, @plan_id, @state, @current_period_start, @current_period_end)
				 RETURNING id`,
				pgx.NamedArgs{
					"tenant_id":            tenantID,
					"plan_id":              planID,
					"state":                state,
					"current_period_start": start,
					"current_period_end":   start.AddDate(0, 1, 0),
				}).Scan(&id)
		}))
	return id
}

// seedActiveSubscription inserts an active subscription with the given billing
// period and returns its id.
func seedActiveSubscription(t *testing.T, pool *pgxpool.Pool, tenantID, planID string,
	periodStart, periodEnd time.Time) string {

	t.Helper()
	var id string
	require.NoError(t, tenantdb.WithTenant(context.Background(), pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx,
				`INSERT INTO tenant_subscriptions
				    (tenant_id, plan_id, state, current_period_start, current_period_end)
				 VALUES (@tenant_id, @plan_id, 'active', @current_period_start, @current_period_end)
				 RETURNING id`,
				pgx.NamedArgs{
					"tenant_id":            tenantID,
					"plan_id":              planID,
					"current_period_start": periodStart,
					"current_period_end":   periodEnd,
				}).Scan(&id)
		}))
	return id
}

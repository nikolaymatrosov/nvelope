package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/billing/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/billing/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

// recordingEnqueuer records the billing.charge jobs a sweep fans out.
type recordingEnqueuer struct {
	calls []string
}

func (e *recordingEnqueuer) EnqueueBillingCharge(_ context.Context, _, subscriptionID string) error {
	e.calls = append(e.calls, subscriptionID)
	return nil
}

// count returns how many times subscriptionID appears in the recorded calls.
func (e *recordingEnqueuer) count(subscriptionID string) int {
	n := 0
	for _, c := range e.calls {
		if c == subscriptionID {
			n++
		}
	}
	return n
}

func TestBillingSweepEnqueuesChargePerDueSubscription(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	planID := seedPlan(t, pool, "published")

	// An active subscription whose period ended an hour ago is due for renewal.
	periodStart := time.Now().UTC().Truncate(time.Second).AddDate(0, -1, 0)
	dueID := seedActiveSubscription(t, pool, tenantID, planID, periodStart,
		time.Now().UTC().Truncate(time.Second).Add(-time.Hour))

	enqueuer := &recordingEnqueuer{}
	handler := command.NewRunBillingSweepHandler(adapters.NewDueSubscriptions(pool), enqueuer)
	require.NoError(t, handler.Handle(ctx, command.RunBillingSweep{}))

	// The due subscription is enqueued exactly once — the sweep never stacks a
	// duplicate charge for the same subscription.
	require.Equal(t, 1, enqueuer.count(dueID))
}

func TestBillingSweepSkipsCurrentSubscription(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	planID := seedPlan(t, pool, "published")

	// An active subscription whose period ends in the future is not due.
	periodStart := time.Now().UTC().Truncate(time.Second)
	notDueID := seedActiveSubscription(t, pool, tenantID, planID, periodStart,
		periodStart.AddDate(0, 1, 0))

	enqueuer := &recordingEnqueuer{}
	handler := command.NewRunBillingSweepHandler(adapters.NewDueSubscriptions(pool), enqueuer)
	require.NoError(t, handler.Handle(ctx, command.RunBillingSweep{}))

	require.Equal(t, 0, enqueuer.count(notDueID))
}

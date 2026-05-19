package command_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/billing/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/billing/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/billing/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

func TestCancelSubscriptionFlagsForPeriodEnd(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	planID := seedPlan(t, pool, "published")

	subscribe := newSubscribeHandler(pool, adapters.NewMockGateway())
	_, err := subscribe.Handle(ctx, command.Subscribe{TenantID: tenantID, ActorID: "actor", PlanID: planID})
	require.NoError(t, err)

	subs := adapters.NewSubscriptions(pool)
	cancel := command.NewCancelSubscriptionHandler(subs, noopAudit{})
	require.NoError(t, cancel.Handle(ctx, command.CancelSubscription{TenantID: tenantID, ActorID: "actor"}))

	// The subscription stays active but is flagged to cancel at period end.
	sub, found, err := subs.Current(ctx, tenantID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, domain.SubscriptionActive, sub.State())
	require.True(t, sub.CancelAtPeriodEnd())
}

func TestCancelledSubscriptionTerminatesAtPeriodEnd(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	planID := seedPlan(t, pool, "published")

	subscribe := newSubscribeHandler(pool, adapters.NewMockGateway())
	_, err := subscribe.Handle(ctx, command.Subscribe{TenantID: tenantID, ActorID: "actor", PlanID: planID})
	require.NoError(t, err)

	subs := adapters.NewSubscriptions(pool)
	cancel := command.NewCancelSubscriptionHandler(subs, noopAudit{})
	require.NoError(t, cancel.Handle(ctx, command.CancelSubscription{TenantID: tenantID, ActorID: "actor"}))

	sub, _, err := subs.Current(ctx, tenantID)
	require.NoError(t, err)

	// Force the current period into the past, then run the charge path: a
	// subscription flagged for cancellation is terminated, not renewed.
	require.NoError(t, tenantdb.WithTenant(ctx, pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`UPDATE tenant_subscriptions SET current_period_end = now() - interval '1 hour'
			 WHERE id = $1`, sub.ID())
		return err
	}))

	charge := command.NewChargeInvoiceHandler(subs, adapters.NewInvoices(pool),
		adapters.NewPlans(pool), adapters.NewMockGateway(), domain.NewDunningPolicy(3, 72*time.Hour))
	_, err = charge.Handle(ctx, command.ChargeInvoice{TenantID: tenantID, SubscriptionID: sub.ID()})
	require.NoError(t, err)

	got, err := subs.Get(ctx, tenantID, sub.ID())
	require.NoError(t, err)
	require.Equal(t, domain.SubscriptionCanceled, got.State())
	require.NotNil(t, got.CanceledAt())
}

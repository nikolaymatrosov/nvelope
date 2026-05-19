package command_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/billing/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/billing/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/billing/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

// suspendViaDunning subscribes the tenant with a declining gateway and exhausts
// its dunning retries, leaving the subscription suspended. It returns the
// subscription id and the invoice id of the now-uncollectible invoice.
func suspendViaDunning(t *testing.T, pool *pgxpool.Pool, gateway *adapters.MockGateway,
	tenantID, planID string) (string, string) {

	t.Helper()
	ctx := context.Background()
	subs := adapters.NewSubscriptions(pool)
	invs := adapters.NewInvoices(pool)
	charge := command.NewChargeInvoiceHandler(subs, invs, adapters.NewPlans(pool),
		gateway, domain.NewDunningPolicy(3, 72*time.Hour))
	subscribe := command.NewSubscribeHandler(adapters.NewPlans(pool), subs, invs, charge, noopAudit{})

	gateway.DeclineTenant(tenantID, "card_declined")
	_, err := subscribe.Handle(ctx, command.Subscribe{
		TenantID: tenantID, ActorID: "actor", PlanID: planID,
	})
	require.ErrorIs(t, err, domain.ErrPaymentFailed)

	sub, _, err := subs.Current(ctx, tenantID)
	require.NoError(t, err)
	for range []struct{}{{}, {}} {
		_, err := charge.Handle(ctx, command.ChargeInvoice{
			TenantID: tenantID, SubscriptionID: sub.ID(),
		})
		require.NoError(t, err)
	}
	sub, _, err = subs.Current(ctx, tenantID)
	require.NoError(t, err)
	require.Equal(t, domain.SubscriptionSuspended, sub.State())

	inv, found, err := invs.BySubscriptionPeriod(ctx, tenantID, sub.ID(), sub.CurrentPeriodStart())
	require.NoError(t, err)
	require.True(t, found)
	return sub.ID(), inv.ID()
}

func TestSettleInvoiceReinstatesSuspendedSubscription(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	planID := seedPlan(t, pool, "published")

	gateway := adapters.NewMockGateway()
	subID, invID := suspendViaDunning(t, pool, gateway, tenantID, planID)

	// The gateway recovers; settling the invoice clears the balance.
	gateway.Reset()
	charge := command.NewChargeInvoiceHandler(adapters.NewSubscriptions(pool),
		adapters.NewInvoices(pool), adapters.NewPlans(pool), gateway,
		domain.NewDunningPolicy(3, 72*time.Hour))
	settle := command.NewSettleInvoiceHandler(charge, noopAudit{})

	res, err := settle.Handle(ctx, command.SettleInvoice{
		TenantID: tenantID, ActorID: "actor", InvoiceID: invID,
	})
	require.NoError(t, err)
	require.Equal(t, invID, res.InvoiceID)

	inv, err := adapters.NewInvoices(pool).Get(ctx, tenantID, invID)
	require.NoError(t, err)
	require.True(t, inv.IsPaid())

	sub, err := adapters.NewSubscriptions(pool).Get(ctx, tenantID, subID)
	require.NoError(t, err)
	require.Equal(t, domain.SubscriptionActive, sub.State())
}

func TestSettleInvoiceDeclinedChangesNothing(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	planID := seedPlan(t, pool, "published")

	gateway := adapters.NewMockGateway()
	subID, invID := suspendViaDunning(t, pool, gateway, tenantID, planID)

	// The gateway still declines — the settle fails and nothing changes.
	charge := command.NewChargeInvoiceHandler(adapters.NewSubscriptions(pool),
		adapters.NewInvoices(pool), adapters.NewPlans(pool), gateway,
		domain.NewDunningPolicy(3, 72*time.Hour))
	settle := command.NewSettleInvoiceHandler(charge, noopAudit{})

	_, err := settle.Handle(ctx, command.SettleInvoice{
		TenantID: tenantID, ActorID: "actor", InvoiceID: invID,
	})
	require.ErrorIs(t, err, domain.ErrPaymentFailed)

	sub, err := adapters.NewSubscriptions(pool).Get(ctx, tenantID, subID)
	require.NoError(t, err)
	require.Equal(t, domain.SubscriptionSuspended, sub.State())
}

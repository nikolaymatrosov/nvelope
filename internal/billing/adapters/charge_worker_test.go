package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/billing/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/billing/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/billing/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

func TestChargeRenewsDueSubscription(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	planID := seedPlan(t, pool, "published")

	subs := adapters.NewSubscriptions(pool)
	invs := adapters.NewInvoices(pool)
	handler := command.NewChargeInvoiceHandler(subs, invs, adapters.NewPlans(pool), adapters.NewMockGateway(), domain.NewDunningPolicy(3, 72*time.Hour))

	// An active subscription whose period ended an hour ago.
	periodEnd := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)
	periodStart := periodEnd.AddDate(0, -1, 0)
	subID := seedActiveSubscription(t, pool, tenantID, planID, periodStart, periodEnd)

	res, err := handler.Handle(ctx, command.ChargeInvoice{TenantID: tenantID, SubscriptionID: subID})
	require.NoError(t, err)
	require.True(t, res.Succeeded)

	// A renewal invoice for the new period was generated and paid.
	inv, found, err := invs.BySubscriptionPeriod(ctx, tenantID, subID, periodEnd)
	require.NoError(t, err)
	require.True(t, found)
	require.True(t, inv.IsPaid())

	// The subscription advanced into the next period and stays active.
	sub, err := subs.Get(ctx, tenantID, subID)
	require.NoError(t, err)
	require.Equal(t, domain.SubscriptionActive, sub.State())
	require.True(t, sub.CurrentPeriodEnd().After(time.Now().UTC()))
}

func TestChargeIsIdempotent(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	planID := seedPlan(t, pool, "published")

	subs := adapters.NewSubscriptions(pool)
	invs := adapters.NewInvoices(pool)
	handler := command.NewChargeInvoiceHandler(subs, invs, adapters.NewPlans(pool), adapters.NewMockGateway(), domain.NewDunningPolicy(3, 72*time.Hour))

	subID := seedSubscription(t, pool, tenantID, planID)
	period := time.Now().UTC().Truncate(time.Second)
	invID, err := invs.Add(ctx, newInvoice(t, tenantID, subID, period))
	require.NoError(t, err)

	// Charge, then charge again — a retried billing.charge must not double-charge.
	_, err = handler.Handle(ctx, command.ChargeInvoice{TenantID: tenantID, SubscriptionID: subID})
	require.NoError(t, err)
	_, err = handler.Handle(ctx, command.ChargeInvoice{TenantID: tenantID, SubscriptionID: subID})
	require.NoError(t, err)

	got, err := invs.Get(ctx, tenantID, invID)
	require.NoError(t, err)
	require.True(t, got.IsPaid())

	attempts, err := invs.Attempts(ctx, tenantID, invID)
	require.NoError(t, err)
	require.Len(t, attempts, 1, "exactly one successful charge despite the retry")
}

func TestChargeDunningPathSuspends(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	planID := seedPlan(t, pool, "published")

	subs := adapters.NewSubscriptions(pool)
	invs := adapters.NewInvoices(pool)
	gateway := adapters.NewMockGateway()
	gateway.DeclineTenant(tenantID, "card_declined")
	handler := command.NewChargeInvoiceHandler(subs, invs, adapters.NewPlans(pool),
		gateway, domain.NewDunningPolicy(3, 72*time.Hour))

	subID := seedSubscription(t, pool, tenantID, planID)
	period := time.Now().UTC().Truncate(time.Second)
	invID, err := invs.Add(ctx, newInvoice(t, tenantID, subID, period))
	require.NoError(t, err)

	// First failed charge: the subscription enters the dunning grace window
	// and the next retry is scheduled.
	_, err = handler.Handle(ctx, command.ChargeInvoice{TenantID: tenantID, SubscriptionID: subID})
	require.NoError(t, err)
	inv, err := invs.Get(ctx, tenantID, invID)
	require.NoError(t, err)
	require.Equal(t, domain.InvoiceOpen, inv.Status())
	require.Equal(t, 1, inv.AttemptCount())
	require.NotNil(t, inv.NextAttemptAt())
	sub, err := subs.Get(ctx, tenantID, subID)
	require.NoError(t, err)
	require.Equal(t, domain.SubscriptionPastDue, sub.State())

	// Two more failed charges exhaust the retries.
	_, err = handler.Handle(ctx, command.ChargeInvoice{TenantID: tenantID, SubscriptionID: subID})
	require.NoError(t, err)
	_, err = handler.Handle(ctx, command.ChargeInvoice{TenantID: tenantID, SubscriptionID: subID})
	require.NoError(t, err)

	inv, err = invs.Get(ctx, tenantID, invID)
	require.NoError(t, err)
	require.Equal(t, domain.InvoiceUncollectible, inv.Status())
	require.Equal(t, 3, inv.AttemptCount())

	sub, err = subs.Get(ctx, tenantID, subID)
	require.NoError(t, err)
	require.Equal(t, domain.SubscriptionSuspended, sub.State())
}

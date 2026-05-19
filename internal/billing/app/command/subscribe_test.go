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

// noopAudit is an AuditWriter that records nothing — the audit_log row is not
// the subject of these tests.
type noopAudit struct{}

func (noopAudit) Record(_ context.Context, _, _, _, _ string) error { return nil }

// seedTenant inserts a tenant row and returns its id.
func seedTenant(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO tenants (name, slug, status) VALUES ('Workspace', $1, 'active') RETURNING id`,
		"sub-"+dbtest.RandString()).Scan(&id))
	return id
}

// seedPlan inserts a plan with the given status and returns its id.
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

// newSubscribeHandler wires the Subscribe handler over real repositories and
// the given gateway.
func newSubscribeHandler(pool *pgxpool.Pool, gateway domain.PaymentGateway) command.SubscribeHandler {
	plans := adapters.NewPlans(pool)
	subscriptions := adapters.NewSubscriptions(pool)
	invoices := adapters.NewInvoices(pool)
	charge := command.NewChargeInvoiceHandler(subscriptions, invoices, plans, gateway,
		domain.NewDunningPolicy(3, 72*time.Hour))
	return command.NewSubscribeHandler(plans, subscriptions, invoices, charge, noopAudit{})
}

func TestSubscribeSucceeds(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	planID := seedPlan(t, pool, "published")
	h := newSubscribeHandler(pool, adapters.NewMockGateway())

	res, err := h.Handle(ctx, command.Subscribe{TenantID: tenantID, ActorID: "actor", PlanID: planID})
	require.NoError(t, err)
	require.Equal(t, string(domain.SubscriptionActive), res.State)
	require.Equal(t, string(domain.InvoicePaid), res.InvoiceStatus)
	require.Equal(t, int64(990000), res.InvoiceTotalMinor)

	sub, found, err := adapters.NewSubscriptions(pool).Current(ctx, tenantID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, domain.SubscriptionActive, sub.State())
}

func TestSubscribeGatewayDecline(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	planID := seedPlan(t, pool, "published")

	gateway := adapters.NewMockGateway()
	gateway.DeclineTenant(tenantID, "card_declined")
	h := newSubscribeHandler(pool, gateway)

	_, err := h.Handle(ctx, command.Subscribe{TenantID: tenantID, ActorID: "actor", PlanID: planID})
	require.ErrorIs(t, err, domain.ErrPaymentFailed)

	// The subscription was created but is past_due, and the invoice stays open.
	sub, found, err := adapters.NewSubscriptions(pool).Current(ctx, tenantID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, domain.SubscriptionPastDue, sub.State())

	_, open, err := adapters.NewInvoices(pool).OpenForSubscription(ctx, tenantID, sub.ID())
	require.NoError(t, err)
	require.True(t, open)
}

func TestSubscribeRejectsDuplicate(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	planID := seedPlan(t, pool, "published")
	h := newSubscribeHandler(pool, adapters.NewMockGateway())

	_, err := h.Handle(ctx, command.Subscribe{TenantID: tenantID, ActorID: "actor", PlanID: planID})
	require.NoError(t, err)

	_, err = h.Handle(ctx, command.Subscribe{TenantID: tenantID, ActorID: "actor", PlanID: planID})
	require.ErrorIs(t, err, domain.ErrSubscriptionExists)
}

func TestSubscribeRejectsUnpublishedPlan(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	planID := seedPlan(t, pool, "draft")
	h := newSubscribeHandler(pool, adapters.NewMockGateway())

	_, err := h.Handle(ctx, command.Subscribe{TenantID: tenantID, ActorID: "actor", PlanID: planID})
	require.ErrorIs(t, err, domain.ErrPlanNotPublished)
}

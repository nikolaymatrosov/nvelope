package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/billing/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/billing/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

// newInvoice builds an open invoice with a single subscription line item.
func newInvoice(t *testing.T, tenantID, subscriptionID string, periodStart time.Time) *domain.Invoice {
	t.Helper()
	li := domain.NewLineItem(domain.LineItemSubscription, "Starter subscription", 1,
		domain.NewMoney(990000, "RUB"))
	inv, err := domain.NewInvoice(tenantID, subscriptionID, periodStart,
		periodStart.AddDate(0, 1, 0), "RUB", []*domain.InvoiceLineItem{li})
	require.NoError(t, err)
	return inv
}

func TestInvoicesAddAndGet(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewInvoices(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	planID := seedPlan(t, pool, "published")
	subID := seedSubscription(t, pool, tenantID, planID)
	period := time.Now().UTC().Truncate(time.Second)

	id, err := repo.Add(ctx, newInvoice(t, tenantID, subID, period))
	require.NoError(t, err)

	got, err := repo.Get(ctx, tenantID, id)
	require.NoError(t, err)
	require.Equal(t, domain.InvoiceOpen, got.Status())
	require.Equal(t, int64(990000), got.Total().Minor())
	require.Len(t, got.LineItems(), 1)
	require.Equal(t, domain.LineItemSubscription, got.LineItems()[0].Kind())
}

func TestInvoicesAddOrGetIdempotent(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewInvoices(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	planID := seedPlan(t, pool, "published")
	subID := seedSubscription(t, pool, tenantID, planID)
	period := time.Now().UTC().Truncate(time.Second)

	first, created, err := repo.AddOrGet(ctx, newInvoice(t, tenantID, subID, period))
	require.NoError(t, err)
	require.True(t, created)

	// A second invoice for the same (subscription, period) loads the existing
	// row instead of inserting a duplicate.
	second, created, err := repo.AddOrGet(ctx, newInvoice(t, tenantID, subID, period))
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, first.ID(), second.ID())
}

func TestInvoicesPaymentAttempts(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewInvoices(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	planID := seedPlan(t, pool, "published")
	subID := seedSubscription(t, pool, tenantID, planID)
	period := time.Now().UTC().Truncate(time.Second)
	id, err := repo.Add(ctx, newInvoice(t, tenantID, subID, period))
	require.NoError(t, err)

	next, err := repo.NextAttemptNumber(ctx, tenantID, id)
	require.NoError(t, err)
	require.Equal(t, 1, next)

	require.NoError(t, repo.AddAttempt(ctx,
		domain.NewFailedAttempt(tenantID, id, 1, "card_declined")))
	paid, err := repo.HasSucceededAttempt(ctx, tenantID, id)
	require.NoError(t, err)
	require.False(t, paid)

	next, err = repo.NextAttemptNumber(ctx, tenantID, id)
	require.NoError(t, err)
	require.Equal(t, 2, next)

	require.NoError(t, repo.AddAttempt(ctx,
		domain.NewSucceededAttempt(tenantID, id, 2, "mock_ref")))
	paid, err = repo.HasSucceededAttempt(ctx, tenantID, id)
	require.NoError(t, err)
	require.True(t, paid)

	attempts, err := repo.Attempts(ctx, tenantID, id)
	require.NoError(t, err)
	require.Len(t, attempts, 2)
}

func TestInvoicesUpdateAndList(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewInvoices(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	planID := seedPlan(t, pool, "published")
	subID := seedSubscription(t, pool, tenantID, planID)
	period := time.Now().UTC().Truncate(time.Second)
	id, err := repo.Add(ctx, newInvoice(t, tenantID, subID, period))
	require.NoError(t, err)

	require.NoError(t, repo.Update(ctx, tenantID, id,
		func(i *domain.Invoice) (*domain.Invoice, error) {
			return i, i.MarkPaid(time.Now())
		}))

	got, err := repo.Get(ctx, tenantID, id)
	require.NoError(t, err)
	require.True(t, got.IsPaid())

	open, found, err := repo.OpenForSubscription(ctx, tenantID, subID)
	require.NoError(t, err)
	require.False(t, found, "a paid invoice is no longer open")
	require.Nil(t, open)

	page, total, err := repo.List(ctx, tenantID, 50, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, page, 1)
}

func TestInvoicesCrossTenantIsolation(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewInvoices(pool)
	ctx := context.Background()
	tenantA := seedTenant(t, pool)
	tenantB := seedTenant(t, pool)
	planID := seedPlan(t, pool, "published")
	subA := seedSubscription(t, pool, tenantA, planID)
	period := time.Now().UTC().Truncate(time.Second)

	idA, err := repo.Add(ctx, newInvoice(t, tenantA, subA, period))
	require.NoError(t, err)

	// Tenant B cannot see tenant A's invoice.
	_, err = repo.Get(ctx, tenantB, idA)
	require.ErrorIs(t, err, domain.ErrInvoiceNotFound)

	page, total, err := repo.List(ctx, tenantB, 50, 0)
	require.NoError(t, err)
	require.Equal(t, 0, total)
	require.Empty(t, page)
}

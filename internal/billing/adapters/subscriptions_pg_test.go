package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/billing/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/billing/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

// newSubscription builds a pending subscription for tenant/plan.
func newSubscription(t *testing.T, tenantID, planID string) *domain.Subscription {
	t.Helper()
	start := time.Now().UTC().Truncate(time.Second)
	s, err := domain.NewSubscription(tenantID, planID, start, start.AddDate(0, 1, 0))
	require.NoError(t, err)
	return s
}

// seedSubscription inserts a pending subscription and returns its id.
func seedSubscription(t *testing.T, pool *pgxpool.Pool, tenantID, planID string) string {
	t.Helper()
	repo := adapters.NewSubscriptions(pool)
	id, err := repo.Add(context.Background(), newSubscription(t, tenantID, planID))
	require.NoError(t, err)
	return id
}

func TestSubscriptionsAddGetCurrent(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewSubscriptions(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	planID := seedPlan(t, pool, "published")

	id, err := repo.Add(ctx, newSubscription(t, tenantID, planID))
	require.NoError(t, err)
	require.NotEmpty(t, id)

	got, err := repo.Get(ctx, tenantID, id)
	require.NoError(t, err)
	require.Equal(t, domain.SubscriptionPending, got.State())
	require.Equal(t, planID, got.PlanID())

	current, found, err := repo.Current(ctx, tenantID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, id, current.ID())
}

func TestSubscriptionsUpdateTransition(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewSubscriptions(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	planID := seedPlan(t, pool, "published")
	id := seedSubscription(t, pool, tenantID, planID)

	require.NoError(t, repo.Update(ctx, tenantID, id,
		func(s *domain.Subscription) (*domain.Subscription, error) {
			return s, s.Activate()
		}))

	got, err := repo.Get(ctx, tenantID, id)
	require.NoError(t, err)
	require.Equal(t, domain.SubscriptionActive, got.State())
}

func TestSubscriptionsOneActivePerTenant(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewSubscriptions(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	planID := seedPlan(t, pool, "published")

	_, err := repo.Add(ctx, newSubscription(t, tenantID, planID))
	require.NoError(t, err)

	// A second non-canceled subscription is rejected by the partial unique index.
	_, err = repo.Add(ctx, newSubscription(t, tenantID, planID))
	require.ErrorIs(t, err, domain.ErrSubscriptionExists)
}

func TestSubscriptionsCrossTenantIsolation(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewSubscriptions(pool)
	ctx := context.Background()
	tenantA := seedTenant(t, pool)
	tenantB := seedTenant(t, pool)
	planID := seedPlan(t, pool, "published")

	idA := seedSubscription(t, pool, tenantA, planID)

	// Tenant B has no subscription and cannot see tenant A's.
	_, found, err := repo.Current(ctx, tenantB)
	require.NoError(t, err)
	require.False(t, found)

	_, err = repo.Get(ctx, tenantB, idA)
	require.ErrorIs(t, err, domain.ErrNoSubscription)
}

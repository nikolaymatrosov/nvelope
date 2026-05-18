package adapters_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
)

func TestSuppressionsUpsertAndList(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewSuppressions(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	entry, err := domain.NewManualSuppression(tenantID, "blocked@acme.com", "by support")
	require.NoError(t, err)
	require.NoError(t, repo.Upsert(ctx, entry))
	// A second upsert of the same address is a no-op.
	require.NoError(t, repo.Upsert(ctx, entry))

	page, next, err := repo.List(ctx, tenantID, domain.SuppressionFilter{})
	require.NoError(t, err)
	require.Empty(t, next)
	require.Len(t, page, 1)
	require.Equal(t, "blocked@acme.com", page[0].Email())
	require.Equal(t, domain.ReasonManual, page[0].Reason())
}

func TestSuppressionsRemove(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewSuppressions(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	entry, err := domain.NewManualSuppression(tenantID, "gone@acme.com", "")
	require.NoError(t, err)
	require.NoError(t, repo.Upsert(ctx, entry))

	require.NoError(t, repo.Remove(ctx, tenantID, "gone@acme.com"))
	require.ErrorIs(t, repo.Remove(ctx, tenantID, "gone@acme.com"), domain.ErrSuppressionNotFound)
}

func TestSuppressionsCrossTenantIsolation(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewSuppressions(pool)
	ctx := context.Background()
	tenantA := seedTenant(t, pool)
	tenantB := seedTenant(t, pool)

	entryA, err := domain.NewManualSuppression(tenantA, "shared@acme.com", "")
	require.NoError(t, err)
	require.NoError(t, repo.Upsert(ctx, entryA))

	// The same address is mailable for tenant B — its list is empty.
	pageB, _, err := repo.List(ctx, tenantB, domain.SuppressionFilter{})
	require.NoError(t, err)
	require.Empty(t, pageB)
	// Tenant B cannot remove tenant A's entry.
	require.ErrorIs(t, repo.Remove(ctx, tenantB, "shared@acme.com"), domain.ErrSuppressionNotFound)

	pageA, _, err := repo.List(ctx, tenantA, domain.SuppressionFilter{})
	require.NoError(t, err)
	require.Len(t, pageA, 1)
}

func TestSettingsGetDefaultAndPut(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewSettings(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	// No row yet — the defaults apply (both toggles on).
	got, err := repo.Get(ctx, tenantID)
	require.NoError(t, err)
	require.True(t, got.SuppressOnHardBounce())
	require.True(t, got.SuppressOnComplaint())

	require.NoError(t, repo.Put(ctx, tenantID, domain.NewBounceSettings(tenantID, true, false)))
	got, err = repo.Get(ctx, tenantID)
	require.NoError(t, err)
	require.True(t, got.SuppressOnHardBounce())
	require.False(t, got.SuppressOnComplaint())
}

func TestSettingsCrossTenantIsolation(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewSettings(pool)
	ctx := context.Background()
	tenantA := seedTenant(t, pool)
	tenantB := seedTenant(t, pool)

	require.NoError(t, repo.Put(ctx, tenantA, domain.NewBounceSettings(tenantA, false, false)))

	// Tenant B is unaffected — it still sees the defaults.
	gotB, err := repo.Get(ctx, tenantB)
	require.NoError(t, err)
	require.True(t, gotB.SuppressOnHardBounce())
	require.True(t, gotB.SuppressOnComplaint())
}

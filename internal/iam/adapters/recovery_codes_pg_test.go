package adapters_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/iam/adapters"
)

func TestRecoveryCodesAddBatchAndConsume(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewRecoveryCodes(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	userID := seedTenantUser(t, pool, tenantID)

	require.NoError(t, repo.AddBatch(ctx, tenantID, userID, []string{"h1", "h2", "h3"}))

	consumed, err := repo.Consume(ctx, tenantID, userID, "h2")
	require.NoError(t, err)
	require.True(t, consumed, "an unused code is consumable")

	consumed, err = repo.Consume(ctx, tenantID, userID, "h2")
	require.NoError(t, err)
	require.False(t, consumed, "a code cannot be consumed twice")

	consumed, err = repo.Consume(ctx, tenantID, userID, "unknown")
	require.NoError(t, err)
	require.False(t, consumed, "an unknown code is not consumable")
}

func TestRecoveryCodesAddBatchReplacesPriorCodes(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewRecoveryCodes(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	userID := seedTenantUser(t, pool, tenantID)

	require.NoError(t, repo.AddBatch(ctx, tenantID, userID, []string{"old"}))
	require.NoError(t, repo.AddBatch(ctx, tenantID, userID, []string{"new"}))

	consumed, err := repo.Consume(ctx, tenantID, userID, "old")
	require.NoError(t, err)
	require.False(t, consumed, "a re-issued batch invalidates the prior codes")

	consumed, err = repo.Consume(ctx, tenantID, userID, "new")
	require.NoError(t, err)
	require.True(t, consumed)
}

func TestRecoveryCodesDeleteForUser(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewRecoveryCodes(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	userID := seedTenantUser(t, pool, tenantID)

	require.NoError(t, repo.AddBatch(ctx, tenantID, userID, []string{"h1"}))
	require.NoError(t, repo.DeleteForUser(ctx, tenantID, userID))

	consumed, err := repo.Consume(ctx, tenantID, userID, "h1")
	require.NoError(t, err)
	require.False(t, consumed)
}

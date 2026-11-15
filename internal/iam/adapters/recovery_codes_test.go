package adapters_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRecoveryCodesAddBatchAndConsume(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.recoveryCodes(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		userID := newTenantUser(t, b, tenantID)

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

		consumed, err = repo.Consume(ctx, tenantID, userID, "h1")
		require.NoError(t, err)
		require.True(t, consumed, "consuming one code leaves the others usable")
	})
}

func TestRecoveryCodesAddBatchReplacesPriorCodes(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.recoveryCodes(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		userID := newTenantUser(t, b, tenantID)

		require.NoError(t, repo.AddBatch(ctx, tenantID, userID, []string{"old", "kept"}))
		consumed, err := repo.Consume(ctx, tenantID, userID, "kept")
		require.NoError(t, err)
		require.True(t, consumed)

		require.NoError(t, repo.AddBatch(ctx, tenantID, userID, []string{"new", "kept"}))

		consumed, err = repo.Consume(ctx, tenantID, userID, "old")
		require.NoError(t, err)
		require.False(t, consumed, "a re-issued batch invalidates the prior codes")

		consumed, err = repo.Consume(ctx, tenantID, userID, "new")
		require.NoError(t, err)
		require.True(t, consumed)

		consumed, err = repo.Consume(ctx, tenantID, userID, "kept")
		require.NoError(t, err)
		require.True(t, consumed, "a code re-issued in the new batch is unused again")
	})
}

func TestRecoveryCodesDeleteForUser(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.recoveryCodes(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		userID := newTenantUser(t, b, tenantID)
		otherUser := newTenantUser(t, b, tenantID)

		require.NoError(t, repo.AddBatch(ctx, tenantID, userID, []string{"h1"}))
		require.NoError(t, repo.AddBatch(ctx, tenantID, otherUser, []string{"h1"}))
		require.NoError(t, repo.DeleteForUser(ctx, tenantID, userID))

		consumed, err := repo.Consume(ctx, tenantID, userID, "h1")
		require.NoError(t, err)
		require.False(t, consumed)

		consumed, err = repo.Consume(ctx, tenantID, otherUser, "h1")
		require.NoError(t, err)
		require.True(t, consumed, "another user's codes are untouched")
	})
}

func TestRecoveryCodesAreTenantScoped(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.recoveryCodes(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		otherTenant := b.newTenant(t)
		userID := newTenantUser(t, b, tenantID)

		require.NoError(t, repo.AddBatch(ctx, tenantID, userID, []string{"h1"}))

		consumed, err := repo.Consume(ctx, otherTenant, userID, "h1")
		require.NoError(t, err)
		require.False(t, consumed, "another tenant cannot consume the code")
		require.NoError(t, repo.DeleteForUser(ctx, otherTenant, userID))

		consumed, err = repo.Consume(ctx, tenantID, userID, "h1")
		require.NoError(t, err)
		require.True(t, consumed, "another tenant cannot delete the code")
	})
}

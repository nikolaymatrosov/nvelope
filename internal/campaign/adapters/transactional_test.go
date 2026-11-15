package adapters_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

func TestTransactionalMessagesRecordRoundTrips(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.transactional(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		pm := "pm-" + dbtest.RandString()

		require.NoError(t, repo.Record(ctx, tenantID, "", pm, "user@acme.com"))
		require.Equal(t, 1, b.transactionalCount(t, tenantID))

		// A provider message id is recorded once, within and across tenants.
		require.Error(t, repo.Record(ctx, tenantID, "", pm, "again@acme.com"))
		require.Equal(t, 1, b.transactionalCount(t, tenantID))
		other := b.newTenant(t)
		require.Error(t, repo.Record(ctx, other, "", pm, "again@acme.com"))
		require.Zero(t, b.transactionalCount(t, other))
	})
}

func TestTransactionalMessagesCrossTenantIsolation(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.transactional(t)
		ctx := context.Background()
		tenantA, tenantB := b.newTenant(t), b.newTenant(t)

		require.NoError(t, repo.Record(ctx, tenantA, "", "pm-"+dbtest.RandString(), "a@acme.com"))

		require.Equal(t, 1, b.transactionalCount(t, tenantA))
		require.Equal(t, 0, b.transactionalCount(t, tenantB),
			"tenant B sees none of tenant A's transactional messages")
	})
}

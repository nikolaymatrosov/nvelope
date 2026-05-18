package adapters_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// countTransactionalMessages counts the tenant's transactional_messages rows
// inside its RLS-bound transaction.
func countTransactionalMessages(t *testing.T, tenantID string) int {
	t.Helper()
	pool := dbtest.AppPool(t)
	var count int
	require.NoError(t, tenantdb.WithTenant(context.Background(), pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, "SELECT count(*) FROM transactional_messages").Scan(&count)
		}))
	return count
}

func TestTransactionalMessagesRecordRoundTrips(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewTransactionalMessages(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	pm := "pm-" + dbtest.RandString()

	require.NoError(t, repo.Record(ctx, tenantID, "", pm, "user@acme.com"))

	var providerMessageID, recipient string
	require.NoError(t, tenantdb.WithTenant(ctx, pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx,
				"SELECT provider_message_id, recipient_email FROM transactional_messages WHERE provider_message_id = $1",
				pm).Scan(&providerMessageID, &recipient)
		}))
	require.Equal(t, pm, providerMessageID)
	require.Equal(t, "user@acme.com", recipient)
}

func TestTransactionalMessagesCrossTenantIsolation(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewTransactionalMessages(pool)
	ctx := context.Background()
	tenantA := seedTenant(t, pool)
	tenantB := seedTenant(t, pool)

	require.NoError(t, repo.Record(ctx, tenantA, "", "pm-"+dbtest.RandString(), "a@acme.com"))

	require.Equal(t, 1, countTransactionalMessages(t, tenantA))
	require.Equal(t, 0, countTransactionalMessages(t, tenantB),
		"tenant B sees none of tenant A's transactional messages")
}

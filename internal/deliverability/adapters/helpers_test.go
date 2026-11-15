package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
)

// seedTenant inserts a tenant row directly and returns its id.
func seedTenant(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO tenants (name, slug, status) VALUES ('Workspace', $1, 'active') RETURNING id`,
		"dlv-"+dbtest.RandString()).Scan(&id))
	return id
}

// newInbound builds an inbound notification for tests.
func newInbound(t *testing.T, kind domain.EventKind, providerMsgID string) domain.InboundNotification {
	t.Helper()
	n, err := domain.NewInboundNotification("dk-"+dbtest.RandString(), kind,
		providerMsgID, "rcpt@acme.com", time.Now(), []byte(`{"k":"v"}`))
	require.NoError(t, err)
	return n
}

// stageInbound stages n and, so that the shared test database does not pile up
// pending notifications for PendingInbound to wade through, marks it failed
// when the test ends if it is still pending.
func stageInbound(t *testing.T, repo domain.EventRepository, n domain.InboundNotification) string {
	t.Helper()
	ctx := context.Background()
	id, _, err := repo.StageInbound(ctx, n)
	require.NoError(t, err)
	t.Cleanup(func() {
		if got, err := repo.LoadInbound(ctx, id); err == nil && got.Status == domain.InboundPending {
			_ = repo.MarkInbound(ctx, id, domain.InboundFailed)
		}
	})
	return id
}

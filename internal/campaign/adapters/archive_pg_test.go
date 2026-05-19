package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// insertArchivedCampaign stages a campaign row directly so the archive query
// can be exercised without driving a full send lifecycle.
func insertArchivedCampaign(t *testing.T, pool *pgxpool.Pool, tenantID, name string,
	visible bool, archivedAt time.Time) string {
	t.Helper()
	var id string
	require.NoError(t, tenantdb.WithTenant(context.Background(), pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			var archivedAtArg any
			if !archivedAt.IsZero() {
				archivedAtArg = archivedAt
			}
			return tx.QueryRow(ctx,
				`INSERT INTO campaigns (tenant_id, name, subject, body_html, body_text,
				    from_name, from_local_part, status, max_send_errors,
				    archive_visible, archived_at, started_at)
				 VALUES ($1, $2, 'subject', '<p>hi</p>', '', 'Acme', 'hello',
				    'finished', 100, $3, $4, now())
				 RETURNING id`,
				tenantID, name, visible, archivedAtArg).Scan(&id)
		}))
	return id
}

func TestCampaignsArchivedListsNewestFirst(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	repo := adapters.NewCampaigns(pool)

	older := insertArchivedCampaign(t, pool, tenantID, "Old", true, time.Now().Add(-48*time.Hour))
	newer := insertArchivedCampaign(t, pool, tenantID, "New", true, time.Now().Add(-1*time.Hour))
	hidden := insertArchivedCampaign(t, pool, tenantID, "Hidden", false, time.Time{})

	list, total, err := repo.Archived(ctx, tenantID, domain.Page{Limit: 50})
	require.NoError(t, err)
	require.Equal(t, 2, total, "hidden campaign is not counted in the archive")
	require.Len(t, list, 2)
	require.Equal(t, newer, list[0].ID(), "archive is ordered newest-first")
	require.Equal(t, older, list[1].ID())

	// A hidden campaign is still retrievable by id, but the archive query
	// layer filters it out — GetArchivedCampaign reports not-found via the
	// query handler that wraps this repo.
	got, err := repo.Get(ctx, tenantID, hidden)
	require.NoError(t, err)
	require.False(t, got.ArchiveVisible())
}

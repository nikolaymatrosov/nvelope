package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

func TestCampaignsArchivedListsNewestFirst(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		ctx := context.Background()
		tenantID := b.newTenant(t)
		domainID := b.newSendingDomain(t, tenantID)
		repo := b.campaigns(t)

		// publish starts a campaign and puts it in the archive at the given
		// time, or leaves it out of the archive when visible is false.
		publish := func(name string, visible bool, at time.Time) string {
			id, err := repo.Add(ctx, tenantID, newCampaign(t, tenantID, name, domainID))
			require.NoError(t, err)
			require.NoError(t, repo.Update(ctx, tenantID, id,
				func(c *domain.Campaign) (*domain.Campaign, error) {
					if err := c.Start(time.Now()); err != nil {
						return nil, err
					}
					return c, c.SetArchiveVisible(visible, at)
				}))
			return id
		}
		older := publish("Old", true, time.Now().Add(-48*time.Hour))
		newer := publish("New", true, time.Now().Add(-1*time.Hour))
		hidden := publish("Hidden", false, time.Time{})

		list, total, err := repo.Archived(ctx, tenantID, domain.Page{Limit: 50})
		require.NoError(t, err)
		require.Equal(t, 2, total, "hidden campaign is not counted in the archive")
		require.Len(t, list, 2)
		require.Equal(t, newer, list[0].ID(), "archive is ordered newest-first")
		require.Equal(t, older, list[1].ID())
		require.NotNil(t, list[0].ArchivedAt())

		// A hidden campaign is still retrievable by id, but the archive query
		// layer filters it out.
		got, err := repo.Get(ctx, tenantID, hidden)
		require.NoError(t, err)
		require.False(t, got.ArchiveVisible())
	})
}

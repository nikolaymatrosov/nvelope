package adapters_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

func TestCampaignsAddGetUpdate(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.campaigns(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		domainID := b.newSendingDomain(t, tenantID)

		id, err := repo.Add(ctx, tenantID, newCampaign(t, tenantID, "Spring", domainID))
		require.NoError(t, err)

		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, "Spring", got.Name())
		require.Equal(t, domain.CampaignDraft, got.Status())
		require.Equal(t, domainID, got.SendingDomainID())
		require.Nil(t, got.StartedAt())

		require.NoError(t, repo.Update(ctx, tenantID, id,
			func(c *domain.Campaign) (*domain.Campaign, error) {
				return c, c.Start(time.Now())
			}))
		got, err = repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, domain.CampaignRunning, got.Status())
		require.NotNil(t, got.StartedAt())

		err = repo.Update(ctx, tenantID, "00000000-0000-4000-8000-000000000000",
			func(c *domain.Campaign) (*domain.Campaign, error) { return c, nil })
		require.ErrorIs(t, err, domain.ErrCampaignNotFound)
		_, err = repo.Get(ctx, tenantID, "not-a-uuid")
		require.ErrorIs(t, err, domain.ErrCampaignNotFound)
	})
}

func TestCampaignsAllNewestFirst(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.campaigns(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		domainID := b.newSendingDomain(t, tenantID)

		first, err := repo.Add(ctx, tenantID, newCampaign(t, tenantID, "First", domainID))
		require.NoError(t, err)
		time.Sleep(20 * time.Millisecond)
		second, err := repo.Add(ctx, tenantID, newCampaign(t, tenantID, "Second", domainID))
		require.NoError(t, err)

		all, total, err := repo.All(ctx, tenantID, domain.DefaultPage)
		require.NoError(t, err)
		require.Equal(t, 2, total)
		require.Equal(t, []string{second, first}, []string{all[0].ID(), all[1].ID()})

		page, total, err := repo.All(ctx, tenantID, domain.Page{Offset: 1, Limit: 1})
		require.NoError(t, err)
		require.Equal(t, 2, total)
		require.Len(t, page, 1)
		require.Equal(t, first, page[0].ID())
	})
}

func TestCampaignsSaveAndReadTargets(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.campaigns(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		domainID := b.newSendingDomain(t, tenantID)

		id, err := repo.Add(ctx, tenantID, newCampaign(t, tenantID, "Spring", domainID))
		require.NoError(t, err)

		listID := b.newList(t, tenantID)
		segment := json.RawMessage(`{"op":"and","conditions":[]}`)
		require.NoError(t, repo.SaveTargets(ctx, tenantID, id, []domain.Target{
			{ListID: listID},
			{SegmentQuery: segment},
		}))

		targets, err := repo.Targets(ctx, tenantID, id)
		require.NoError(t, err)
		require.Len(t, targets, 2)
		var lists, segments int
		for _, tg := range targets {
			if tg.ListID != "" {
				require.Equal(t, listID, tg.ListID)
				lists++
			} else {
				require.JSONEq(t, string(segment), string(tg.SegmentQuery))
				segments++
			}
		}
		require.Equal(t, 1, lists)
		require.Equal(t, 1, segments)

		// SaveTargets replaces — a second save does not accumulate.
		require.NoError(t, repo.SaveTargets(ctx, tenantID, id, []domain.Target{{ListID: listID}}))
		targets, err = repo.Targets(ctx, tenantID, id)
		require.NoError(t, err)
		require.Len(t, targets, 1)

		// Targeting a list twice collides on UNIQUE (campaign_id, list_id),
		// and the failed save leaves the previous targets in place.
		require.Error(t, repo.SaveTargets(ctx, tenantID, id,
			[]domain.Target{{ListID: listID}, {ListID: listID}}))
		targets, err = repo.Targets(ctx, tenantID, id)
		require.NoError(t, err)
		require.Len(t, targets, 1)

		// A list that does not exist is rejected, as the foreign key does on
		// Postgres, and the previous targets stay.
		require.Error(t, repo.SaveTargets(ctx, tenantID, id,
			[]domain.Target{{ListID: listID}, {ListID: uuid.NewString()}}))
		targets, err = repo.Targets(ctx, tenantID, id)
		require.NoError(t, err)
		require.Len(t, targets, 1)

		// Two segments are not a collision: only a list is unique.
		require.NoError(t, repo.SaveTargets(ctx, tenantID, id,
			[]domain.Target{{SegmentQuery: segment}, {SegmentQuery: segment}}))
		targets, err = repo.Targets(ctx, tenantID, id)
		require.NoError(t, err)
		require.Len(t, targets, 2)
	})
}

func TestCampaignsCrossTenantIsolation(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.campaigns(t)
		ctx := context.Background()
		tenantA, tenantB := b.newTenant(t), b.newTenant(t)
		domainA := b.newSendingDomain(t, tenantA)

		idA, err := repo.Add(ctx, tenantA, newCampaign(t, tenantA, "Spring", domainA))
		require.NoError(t, err)

		_, err = repo.Get(ctx, tenantB, idA)
		require.ErrorIs(t, err, domain.ErrCampaignNotFound)

		err = repo.Update(ctx, tenantB, idA, func(c *domain.Campaign) (*domain.Campaign, error) {
			return c, nil
		})
		require.ErrorIs(t, err, domain.ErrCampaignNotFound)

		all, _, err := repo.All(ctx, tenantB, domain.DefaultPage)
		require.NoError(t, err)
		require.Empty(t, all)
	})
}

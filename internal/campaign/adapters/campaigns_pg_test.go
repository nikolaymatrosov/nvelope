package adapters_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

func TestCampaignsAddGetUpdate(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewCampaigns(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	domainID := seedSendingDomain(t, pool, tenantID, "mail.acme.com")

	id, err := repo.Add(ctx, tenantID, newCampaign(t, tenantID, "Spring", domainID))
	require.NoError(t, err)

	got, err := repo.Get(ctx, tenantID, id)
	require.NoError(t, err)
	require.Equal(t, "Spring", got.Name())
	require.Equal(t, domain.CampaignDraft, got.Status())
	require.Equal(t, domainID, got.SendingDomainID())

	require.NoError(t, repo.Update(ctx, tenantID, id,
		func(c *domain.Campaign) (*domain.Campaign, error) {
			return c, c.Start(time.Now())
		}))
	got, err = repo.Get(ctx, tenantID, id)
	require.NoError(t, err)
	require.Equal(t, domain.CampaignRunning, got.Status())
	require.NotNil(t, got.StartedAt())
}

func TestCampaignsSaveAndReadTargets(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewCampaigns(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	domainID := seedSendingDomain(t, pool, tenantID, "mail.acme.com")

	id, err := repo.Add(ctx, tenantID, newCampaign(t, tenantID, "Spring", domainID))
	require.NoError(t, err)

	listID := seedList(t, pool, tenantID, "L")

	segment := json.RawMessage(`{"op":"and","conditions":[]}`)
	require.NoError(t, repo.SaveTargets(ctx, tenantID, id, []domain.Target{
		{ListID: listID},
		{SegmentQuery: segment},
	}))

	targets, err := repo.Targets(ctx, tenantID, id)
	require.NoError(t, err)
	require.Len(t, targets, 2)

	// SaveTargets replaces — a second save does not accumulate.
	require.NoError(t, repo.SaveTargets(ctx, tenantID, id, []domain.Target{{ListID: listID}}))
	targets, err = repo.Targets(ctx, tenantID, id)
	require.NoError(t, err)
	require.Len(t, targets, 1)
}

func TestCampaignsCrossTenantIsolation(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewCampaigns(pool)
	ctx := context.Background()
	tenantA := seedTenant(t, pool)
	tenantB := seedTenant(t, pool)
	domainA := seedSendingDomain(t, pool, tenantA, "mail.acme.com")

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
}

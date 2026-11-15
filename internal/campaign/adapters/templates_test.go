package adapters_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

func TestTemplatesAddGetAll(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.templates(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		id, err := repo.Add(ctx, tenantID, newTemplate(t, tenantID, "Welcome"))
		require.NoError(t, err)

		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, "Welcome", got.Name())
		require.Equal(t, domain.KindCampaign, got.Kind())

		all, total, err := repo.All(ctx, tenantID, domain.DefaultPage)
		require.NoError(t, err)
		require.Equal(t, 1, total)
		require.Len(t, all, 1)
	})
}

func TestTemplatesDuplicateNameConflicts(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.templates(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		_, err := repo.Add(ctx, tenantID, newTemplate(t, tenantID, "Dup"))
		require.NoError(t, err)
		_, err = repo.Add(ctx, tenantID, newTemplate(t, tenantID, "Dup"))
		require.ErrorIs(t, err, domain.ErrTemplateNameTaken)

		other, err := repo.Add(ctx, tenantID, newTemplate(t, tenantID, "Other"))
		require.NoError(t, err)
		err = repo.Update(ctx, tenantID, other, func(tpl *domain.Template) (*domain.Template, error) {
			return tpl, tpl.Recompose("Dup", "Subject", "<p>b</p>", "")
		})
		require.ErrorIs(t, err, domain.ErrTemplateNameTaken, "renaming onto a taken name conflicts too")
	})
}

func TestTemplatesUpdate(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.templates(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		id, err := repo.Add(ctx, tenantID, newTemplate(t, tenantID, "Old"))
		require.NoError(t, err)
		require.NoError(t, repo.Update(ctx, tenantID, id,
			func(tpl *domain.Template) (*domain.Template, error) {
				return tpl, tpl.Recompose("New", "New subject", "<p>new</p>", "")
			}))

		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, "New", got.Name())
		require.Equal(t, "New subject", got.Subject())

		err = repo.Update(ctx, tenantID, "00000000-0000-4000-8000-000000000000",
			func(tpl *domain.Template) (*domain.Template, error) { return tpl, nil })
		require.ErrorIs(t, err, domain.ErrTemplateNotFound)
	})
}

func TestTemplatesGetMissing(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.templates(t)
		tenantID := b.newTenant(t)

		_, err := repo.Get(context.Background(), tenantID, "not-a-uuid")
		require.ErrorIs(t, err, domain.ErrTemplateNotFound)
	})
}

func TestTemplatesDelete(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.templates(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		id, err := repo.Add(ctx, tenantID, newTemplate(t, tenantID, "Gone"))
		require.NoError(t, err)
		require.NoError(t, repo.Delete(ctx, tenantID, id))
		_, err = repo.Get(ctx, tenantID, id)
		require.ErrorIs(t, err, domain.ErrTemplateNotFound)
		require.ErrorIs(t, repo.Delete(ctx, tenantID, id), domain.ErrTemplateNotFound)
		require.ErrorIs(t, repo.Delete(ctx, tenantID, "not-a-uuid"), domain.ErrTemplateNotFound)
	})
}

// TestTemplatesDeleteUnlinksCampaigns pins the Postgres ON DELETE SET NULL on
// campaigns.template_id: a campaign keeps its content and loses the link.
func TestTemplatesDeleteUnlinksCampaigns(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		templates, campaigns := b.templates(t), b.campaigns(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		domainID := b.newSendingDomain(t, tenantID)

		tplID, err := templates.Add(ctx, tenantID, newTemplate(t, tenantID, "T"))
		require.NoError(t, err)
		c, err := domain.NewCampaign(tenantID, "From template", "Subject", "<p>hi</p>", "Hi",
			"Acme", "news", domainID, tplID, 100)
		require.NoError(t, err)
		campaignID, err := campaigns.Add(ctx, tenantID, c)
		require.NoError(t, err)
		other, err := campaigns.Add(ctx, tenantID, newCampaign(t, tenantID, "Plain", domainID))
		require.NoError(t, err)

		got, err := campaigns.Get(ctx, tenantID, campaignID)
		require.NoError(t, err)
		require.Equal(t, tplID, got.TemplateID())

		require.NoError(t, templates.Delete(ctx, tenantID, tplID))

		got, err = campaigns.Get(ctx, tenantID, campaignID)
		require.NoError(t, err)
		require.Empty(t, got.TemplateID(), "the campaign is unlinked")
		require.Equal(t, "From template", got.Name(), "and keeps its content")
		_, err = campaigns.Get(ctx, tenantID, other)
		require.NoError(t, err)
	})
}

func TestTemplatesCrossTenantIsolation(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.templates(t)
		ctx := context.Background()
		tenantA, tenantB := b.newTenant(t), b.newTenant(t)

		idA, err := repo.Add(ctx, tenantA, newTemplate(t, tenantA, "Welcome"))
		require.NoError(t, err)

		_, err = repo.Get(ctx, tenantB, idA)
		require.ErrorIs(t, err, domain.ErrTemplateNotFound)
		require.ErrorIs(t, repo.Delete(ctx, tenantB, idA), domain.ErrTemplateNotFound)

		all, _, err := repo.All(ctx, tenantB, domain.DefaultPage)
		require.NoError(t, err)
		require.Empty(t, all)

		// The same name is free in another tenant.
		_, err = repo.Add(ctx, tenantB, newTemplate(t, tenantB, "Welcome"))
		require.NoError(t, err)
	})
}

package command_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// seedCampaignTemplate adds a template of the given kind and returns its id.
func seedTemplate(t *testing.T, repo *fakeTemplateRepo, kind domain.Kind) string {
	t.Helper()
	tpl, err := domain.NewTemplate("tenant-1", "T", kind, "Tpl subject", "<p>tpl</p>", "")
	require.NoError(t, err)
	id, err := repo.Add(context.Background(), "tenant-1", tpl)
	require.NoError(t, err)
	return id
}

func TestCreateCampaignInheritsTemplateContent(t *testing.T) {
	t.Parallel()
	campaigns := newFakeCampaignRepo()
	templates := newFakeTemplateRepo()
	tplID := seedTemplate(t, templates, domain.KindCampaign)

	h := command.NewCreateCampaignHandler(campaigns, templates)
	res, err := h.Handle(context.Background(), command.CreateCampaign{
		TenantID: "tenant-1", Name: "Spring", TemplateID: tplID,
		FromName: "Acme", FromLocalPart: "news", SendingDomainID: "dom-1",
		ListIDs: []string{"list-1"},
	})
	require.NoError(t, err)

	stored := campaigns.byID[res.CampaignID]
	require.Equal(t, "Tpl subject", stored.Subject(), "omitted subject inherits from the template")
	require.Equal(t, "<p>tpl</p>", stored.BodyHTML())
	require.Len(t, campaigns.targets[res.CampaignID], 1)
}

func TestCreateCampaignRejectsTransactionalTemplate(t *testing.T) {
	t.Parallel()
	campaigns := newFakeCampaignRepo()
	templates := newFakeTemplateRepo()
	tplID := seedTemplate(t, templates, domain.KindTransactional)

	h := command.NewCreateCampaignHandler(campaigns, templates)
	_, err := h.Handle(context.Background(), command.CreateCampaign{
		TenantID: "tenant-1", Name: "Spring", TemplateID: tplID,
		FromName: "Acme", FromLocalPart: "news", SendingDomainID: "dom-1",
	})
	require.ErrorIs(t, err, domain.ErrTemplateKindMismatch)
}

// addDraftCampaign creates a draft campaign with the given targets and returns
// its id.
func addDraftCampaign(t *testing.T, campaigns *fakeCampaignRepo, domainID string, targets []domain.Target) string {
	t.Helper()
	c, err := domain.NewCampaign("tenant-1", "C", "Subj", "<p>b</p>", "", "Acme", "news",
		domainID, "", 100)
	require.NoError(t, err)
	id, err := campaigns.Add(context.Background(), "tenant-1", c)
	require.NoError(t, err)
	campaigns.targets[id] = targets
	return id
}

func TestStartCampaignSucceeds(t *testing.T) {
	t.Parallel()
	campaigns := newFakeCampaignRepo()
	enq := &fakeCampaignEnqueuer{}
	id := addDraftCampaign(t, campaigns, "dom-1", []domain.Target{{ListID: "list-1"}})

	h := command.NewStartCampaignHandler(campaigns, fakeDomainLookup{verified: true}, enq)
	require.NoError(t, h.Handle(context.Background(), command.StartCampaign{
		TenantID: "tenant-1", CampaignID: id,
	}))
	require.Equal(t, []string{id}, enq.starts)
	require.Equal(t, domain.CampaignRunning, campaigns.byID[id].Status())
}

func TestStartCampaignRequiresVerifiedDomain(t *testing.T) {
	t.Parallel()
	campaigns := newFakeCampaignRepo()
	enq := &fakeCampaignEnqueuer{}
	id := addDraftCampaign(t, campaigns, "dom-1", []domain.Target{{ListID: "list-1"}})

	h := command.NewStartCampaignHandler(campaigns, fakeDomainLookup{verified: false}, enq)
	err := h.Handle(context.Background(), command.StartCampaign{TenantID: "tenant-1", CampaignID: id})
	require.ErrorIs(t, err, domain.ErrSendingDomainRequired)
	require.Empty(t, enq.starts)
}

func TestStartCampaignRequiresDomain(t *testing.T) {
	t.Parallel()
	campaigns := newFakeCampaignRepo()
	enq := &fakeCampaignEnqueuer{}
	id := addDraftCampaign(t, campaigns, "", []domain.Target{{ListID: "list-1"}})

	h := command.NewStartCampaignHandler(campaigns, fakeDomainLookup{verified: true}, enq)
	err := h.Handle(context.Background(), command.StartCampaign{TenantID: "tenant-1", CampaignID: id})
	require.ErrorIs(t, err, domain.ErrSendingDomainRequired)
}

func TestStartCampaignRequiresTargets(t *testing.T) {
	t.Parallel()
	campaigns := newFakeCampaignRepo()
	enq := &fakeCampaignEnqueuer{}
	id := addDraftCampaign(t, campaigns, "dom-1", nil)

	h := command.NewStartCampaignHandler(campaigns, fakeDomainLookup{verified: true}, enq)
	err := h.Handle(context.Background(), command.StartCampaign{TenantID: "tenant-1", CampaignID: id})
	require.ErrorIs(t, err, domain.ErrCampaignNoRecipients)
}

func TestStartCampaignRejectsNonDraft(t *testing.T) {
	t.Parallel()
	campaigns := newFakeCampaignRepo()
	enq := &fakeCampaignEnqueuer{}
	id := addDraftCampaign(t, campaigns, "dom-1", []domain.Target{{ListID: "list-1"}})

	h := command.NewStartCampaignHandler(campaigns, fakeDomainLookup{verified: true}, enq)
	require.NoError(t, h.Handle(context.Background(), command.StartCampaign{
		TenantID: "tenant-1", CampaignID: id,
	}))
	// A second start is rejected — the campaign is now running.
	err := h.Handle(context.Background(), command.StartCampaign{TenantID: "tenant-1", CampaignID: id})
	require.ErrorIs(t, err, domain.ErrCampaignNotDraft)
}

func TestUpdateCampaignDraftOnly(t *testing.T) {
	t.Parallel()
	campaigns := newFakeCampaignRepo()
	id := addDraftCampaign(t, campaigns, "dom-1", []domain.Target{{ListID: "list-1"}})

	h := command.NewUpdateCampaignHandler(campaigns)
	require.NoError(t, h.Handle(context.Background(), command.UpdateCampaign{
		TenantID: "tenant-1", CampaignID: id, Name: "Renamed", Subject: "S",
		BodyHTML: "<p>x</p>", FromName: "Acme", FromLocalPart: "news", SendingDomainID: "dom-1", ListIDs: []string{"list-1"},
	}))
	require.Equal(t, "Renamed", campaigns.byID[id].Name())

	// Start it, then editing must be rejected.
	require.NoError(t, command.NewStartCampaignHandler(campaigns, fakeDomainLookup{verified: true},
		&fakeCampaignEnqueuer{}).Handle(context.Background(),
		command.StartCampaign{TenantID: "tenant-1", CampaignID: id}))

	err := h.Handle(context.Background(), command.UpdateCampaign{
		TenantID: "tenant-1", CampaignID: id, Name: "Nope", Subject: "S",
		BodyHTML: "<p>x</p>", FromName: "Acme", FromLocalPart: "news", SendingDomainID: "dom-1", ListIDs: []string{"list-1"},
	})
	require.ErrorIs(t, err, domain.ErrCampaignNotEditable)
}

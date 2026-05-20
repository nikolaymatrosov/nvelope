package command_test

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// fakeCampaignRepo is an in-memory CampaignRepository for command unit tests.
type fakeCampaignRepo struct {
	byID    map[string]*domain.Campaign
	targets map[string][]domain.Target
	nextID  int
}

func newFakeCampaignRepo() *fakeCampaignRepo {
	return &fakeCampaignRepo{
		byID:    map[string]*domain.Campaign{},
		targets: map[string][]domain.Target{},
	}
}

func (r *fakeCampaignRepo) Add(_ context.Context, tenantID string, c *domain.Campaign) (string, error) {
	r.nextID++
	id := "camp-" + string(rune('a'+r.nextID))
	r.byID[id] = domain.HydrateCampaign(id, tenantID, c.Name(), c.Subject(), c.BodyHTML(),
		c.BodyText(), c.FromName(), c.FromLocalPart(), c.SendingDomainID(), c.TemplateID(),
		c.Status(), c.MaxSendErrors(), 0, 0, 0, c.BodyDoc(), c.Theme(),
		c.CreatedAt(), c.UpdatedAt(), nil, nil, false, nil)
	return id, nil
}

func (r *fakeCampaignRepo) Get(_ context.Context, _, id string) (*domain.Campaign, error) {
	c, ok := r.byID[id]
	if !ok {
		return nil, domain.ErrCampaignNotFound
	}
	return c, nil
}

func (r *fakeCampaignRepo) Update(_ context.Context, _, id string,
	fn func(*domain.Campaign) (*domain.Campaign, error)) error {
	c, ok := r.byID[id]
	if !ok {
		return domain.ErrCampaignNotFound
	}
	updated, err := fn(c)
	if err != nil {
		return err
	}
	r.byID[id] = updated
	return nil
}

func (r *fakeCampaignRepo) All(context.Context, string, domain.Page) ([]*domain.Campaign, int, error) {
	return nil, 0, nil
}

func (r *fakeCampaignRepo) Archived(context.Context, string, domain.Page) ([]*domain.Campaign, int, error) {
	return nil, 0, nil
}

func (r *fakeCampaignRepo) SaveTargets(_ context.Context, _, campaignID string, ts []domain.Target) error {
	r.targets[campaignID] = ts
	return nil
}

func (r *fakeCampaignRepo) Targets(_ context.Context, _, campaignID string) ([]domain.Target, error) {
	return r.targets[campaignID], nil
}

// fakeTemplateRepo is an in-memory TemplateRepository for command unit tests.
type fakeTemplateRepo struct {
	byID   map[string]*domain.Template
	nextID int
}

func newFakeTemplateRepo() *fakeTemplateRepo {
	return &fakeTemplateRepo{byID: map[string]*domain.Template{}}
}

func (r *fakeTemplateRepo) Add(_ context.Context, tenantID string, t *domain.Template) (string, error) {
	r.nextID++
	id := "tpl-" + string(rune('a'+r.nextID))
	r.byID[id] = domain.HydrateTemplate(id, tenantID, t.Name(), t.Kind(), t.Subject(),
		t.BodyHTML(), t.BodyText(), t.BodyDoc(), t.Theme(), t.CreatedAt(), t.UpdatedAt())
	return id, nil
}

func (r *fakeTemplateRepo) Get(_ context.Context, _, id string) (*domain.Template, error) {
	t, ok := r.byID[id]
	if !ok {
		return nil, domain.ErrTemplateNotFound
	}
	return t, nil
}

func (r *fakeTemplateRepo) Update(_ context.Context, _, id string,
	fn func(*domain.Template) (*domain.Template, error)) error {
	t, ok := r.byID[id]
	if !ok {
		return domain.ErrTemplateNotFound
	}
	updated, err := fn(t)
	if err != nil {
		return err
	}
	r.byID[id] = updated
	return nil
}

func (r *fakeTemplateRepo) All(context.Context, string, domain.Page) ([]*domain.Template, int, error) {
	return nil, 0, nil
}

func (r *fakeTemplateRepo) Delete(_ context.Context, _, id string) error {
	if _, ok := r.byID[id]; !ok {
		return domain.ErrTemplateNotFound
	}
	delete(r.byID, id)
	return nil
}

// fakeDomainLookup is a deterministic SendingDomainLookup.
type fakeDomainLookup struct {
	verified bool
}

func (f fakeDomainLookup) DomainName(context.Context, string, string) (string, error) {
	return "mail.acme.com", nil
}

func (f fakeDomainLookup) IsVerified(context.Context, string, string) (bool, error) {
	return f.verified, nil
}

// stubSuppression is a SuppressionChecker that suppresses the addresses in
// blocked; the zero value suppresses nothing.
type stubSuppression struct{ blocked map[string]string }

func (s stubSuppression) Suppressed(_ context.Context, _ string, emails []string) (
	map[string]string, error) {

	out := map[string]string{}
	for _, e := range emails {
		if reason, ok := s.blocked[e]; ok {
			out[e] = reason
		}
	}
	return out, nil
}

// fakeCampaignEnqueuer records EnqueueStart calls.
type fakeCampaignEnqueuer struct {
	starts []string
}

func (e *fakeCampaignEnqueuer) EnqueueStart(_ context.Context, _, campaignID string) error {
	e.starts = append(e.starts, campaignID)
	return nil
}

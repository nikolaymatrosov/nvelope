package command

import (
	"context"
	"strings"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// CampaignEnqueuer hands a campaign to the durable queue to begin sending. It
// is declared here, by the command layer that depends on it, and implemented
// by the platform/jobs River adapter.
type CampaignEnqueuer interface {
	EnqueueStart(ctx context.Context, tenantID, campaignID string) error
}

// CreateCampaign is the request to create a campaign, optionally from a
// template. When TemplateID is set, omitted content fields inherit from it.
type CreateCampaign struct {
	TenantID        string
	Name            string
	TemplateID      string
	Subject         string
	BodyHTML        string
	BodyText        string
	FromName        string
	FromLocalPart   string
	SendingDomainID string
	ListIDs         []string
	Segments        [][]byte
	MaxSendErrors   int
}

// CreateCampaignResult carries the new campaign's id.
type CreateCampaignResult struct {
	CampaignID string
	Warnings   []domain.RenderWarning
}

// CreateCampaignHandler handles the CreateCampaign command.
type CreateCampaignHandler struct {
	campaigns domain.CampaignRepository
	templates domain.TemplateRepository
	policy    RawHTMLPolicy
}

// NewCreateCampaignHandler builds the handler, failing fast on a nil dependency.
func NewCreateCampaignHandler(campaigns domain.CampaignRepository,
	templates domain.TemplateRepository, policy RawHTMLPolicy) CreateCampaignHandler {
	if campaigns == nil || templates == nil || policy == nil {
		panic("nil dependency")
	}
	return CreateCampaignHandler{campaigns: campaigns, templates: templates, policy: policy}
}

// Handle inherits any omitted content from the origin template, validates the
// campaign, and persists it with its targets. When the template is
// visually-authored (BodyDocJSON is non-nil), the campaign also inherits
// the structured document and the operator's pinned theme override so the
// campaign editor opens visually (per T076).
func (h CreateCampaignHandler) Handle(ctx context.Context, cmd CreateCampaign) (CreateCampaignResult, error) {
	subject, bodyHTML, bodyText := cmd.Subject, cmd.BodyHTML, cmd.BodyText
	var tplBodyDocJSON, tplThemeJSON []byte
	if cmd.TemplateID != "" {
		tpl, err := h.templates.Get(ctx, cmd.TenantID, cmd.TemplateID)
		if err != nil {
			return CreateCampaignResult{}, err
		}
		if tpl.Kind() != domain.KindCampaign {
			return CreateCampaignResult{}, domain.ErrTemplateKindMismatch.WithMessage(
				"a campaign can only be built from a campaign template")
		}
		if strings.TrimSpace(subject) == "" {
			subject = tpl.Subject()
		}
		if strings.TrimSpace(bodyHTML) == "" {
			bodyHTML = tpl.BodyHTML()
		}
		if strings.TrimSpace(bodyText) == "" {
			bodyText = tpl.BodyText()
		}
		tplBodyDocJSON = tpl.BodyDocJSON()
		tplThemeJSON = tpl.ThemeJSON()
	}

	bodyHTML, warnings, err := applyRawHTMLPolicy(ctx, h.policy, cmd.TenantID, bodyHTML)
	if err != nil {
		return CreateCampaignResult{}, err
	}
	c, err := domain.NewCampaign(cmd.TenantID, cmd.Name, subject, bodyHTML, bodyText,
		cmd.FromName, cmd.FromLocalPart, cmd.SendingDomainID, cmd.TemplateID, cmd.MaxSendErrors)
	if err != nil {
		return CreateCampaignResult{}, err
	}
	if len(tplBodyDocJSON) > 0 || len(tplThemeJSON) > 0 {
		c.AttachVisualContent(tplBodyDocJSON, tplThemeJSON)
	}
	id, err := h.campaigns.Add(ctx, cmd.TenantID, c)
	if err != nil {
		return CreateCampaignResult{}, err
	}
	if targets := buildTargets(cmd.ListIDs, cmd.Segments); len(targets) > 0 {
		if err := h.campaigns.SaveTargets(ctx, cmd.TenantID, id, targets); err != nil {
			return CreateCampaignResult{}, err
		}
	}
	return CreateCampaignResult{CampaignID: id, Warnings: warnings}, nil
}

// buildTargets projects raw list ids and segment queries onto domain targets.
func buildTargets(listIDs []string, segments [][]byte) []domain.Target {
	var targets []domain.Target
	for _, listID := range listIDs {
		if listID != "" {
			targets = append(targets, domain.Target{ListID: listID})
		}
	}
	for _, seg := range segments {
		if len(seg) > 0 {
			targets = append(targets, domain.Target{SegmentQuery: seg})
		}
	}
	return targets
}

// UpdateCampaign is the request to change a draft campaign's content and
// targets.
type UpdateCampaign struct {
	TenantID        string
	CampaignID      string
	Name            string
	Subject         string
	BodyHTML        string
	BodyText        string
	FromName        string
	FromLocalPart   string
	SendingDomainID string
	ListIDs         []string
	Segments        [][]byte
}

// UpdateCampaignResult carries the sanitizer's warnings for the saved body.
type UpdateCampaignResult struct {
	Warnings []domain.RenderWarning
}

// UpdateCampaignHandler handles the UpdateCampaign command.
type UpdateCampaignHandler struct {
	campaigns domain.CampaignRepository
	policy    RawHTMLPolicy
}

// NewUpdateCampaignHandler builds the handler, failing fast on a nil dependency.
func NewUpdateCampaignHandler(campaigns domain.CampaignRepository, policy RawHTMLPolicy) UpdateCampaignHandler {
	if campaigns == nil || policy == nil {
		panic("nil dependency")
	}
	return UpdateCampaignHandler{campaigns: campaigns, policy: policy}
}

// Handle sanitizes the raw body per the tenant's policy, applies the new
// content to a draft campaign, and replaces its targets.
func (h UpdateCampaignHandler) Handle(ctx context.Context, cmd UpdateCampaign) (UpdateCampaignResult, error) {
	bodyHTML, warnings, err := applyRawHTMLPolicy(ctx, h.policy, cmd.TenantID, cmd.BodyHTML)
	if err != nil {
		return UpdateCampaignResult{}, err
	}
	if err := h.campaigns.Update(ctx, cmd.TenantID, cmd.CampaignID,
		func(c *domain.Campaign) (*domain.Campaign, error) {
			return c, c.Recompose(cmd.Name, cmd.Subject, bodyHTML, cmd.BodyText,
				cmd.FromName, cmd.FromLocalPart, cmd.SendingDomainID)
		}); err != nil {
		return UpdateCampaignResult{}, err
	}
	if err := h.campaigns.SaveTargets(ctx, cmd.TenantID, cmd.CampaignID,
		buildTargets(cmd.ListIDs, cmd.Segments)); err != nil {
		return UpdateCampaignResult{}, err
	}
	return UpdateCampaignResult{Warnings: warnings}, nil
}

// StartCampaign is the request to begin sending a campaign.
type StartCampaign struct {
	TenantID   string
	CampaignID string
}

// StartCampaignHandler handles the StartCampaign command.
type StartCampaignHandler struct {
	campaigns domain.CampaignRepository
	domains   domain.SendingDomainLookup
	enqueuer  CampaignEnqueuer
	media     MediaPublisher
}

// NewStartCampaignHandler builds the handler, failing fast on a nil dependency.
func NewStartCampaignHandler(campaigns domain.CampaignRepository,
	domains domain.SendingDomainLookup, enqueuer CampaignEnqueuer,
	media MediaPublisher) StartCampaignHandler {
	if campaigns == nil || domains == nil || enqueuer == nil || media == nil {
		panic("nil dependency")
	}
	return StartCampaignHandler{campaigns: campaigns, domains: domains, enqueuer: enqueuer, media: media}
}

// Handle validates the campaign's start preconditions — a draft campaign, a
// verified sending domain, at least one target — then publishes the media its
// body references, transitions it to running, and enqueues the send.
func (h StartCampaignHandler) Handle(ctx context.Context, cmd StartCampaign) error {
	c, err := h.campaigns.Get(ctx, cmd.TenantID, cmd.CampaignID)
	if err != nil {
		return err
	}
	if !c.IsDraft() {
		return domain.ErrCampaignNotDraft
	}
	if c.SendingDomainID() == "" {
		return domain.ErrSendingDomainRequired
	}
	verified, err := h.domains.IsVerified(ctx, cmd.TenantID, c.SendingDomainID())
	if err != nil {
		return err
	}
	if !verified {
		return domain.ErrSendingDomainRequired.WithMessage(
			"the selected sending domain is not verified")
	}
	targets, err := h.campaigns.Targets(ctx, cmd.TenantID, cmd.CampaignID)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return domain.ErrCampaignNoRecipients
	}
	if err := h.campaigns.Update(ctx, cmd.TenantID, cmd.CampaignID,
		func(c *domain.Campaign) (*domain.Campaign, error) {
			// Publish the body that is locked here, so a concurrent draft edit
			// cannot slip in between publishing and starting.
			// ponytail: S3 copies run inside this transaction (a few hundred ms
			// per start); split into publish-then-verify if starts ever contend.
			published, err := h.media.Publish(ctx, cmd.TenantID, c.BodyHTML(), c.BodyText())
			if err != nil {
				return nil, err
			}
			if err := c.PublishBody(published[0], published[1]); err != nil {
				return nil, err
			}
			return c, c.Start(time.Now())
		}); err != nil {
		return err
	}
	return h.enqueuer.EnqueueStart(ctx, cmd.TenantID, cmd.CampaignID)
}

// PauseCampaign is the request to pause a running campaign.
type PauseCampaign struct {
	TenantID   string
	CampaignID string
}

// PauseCampaignHandler handles the PauseCampaign command.
type PauseCampaignHandler struct {
	campaigns domain.CampaignRepository
}

// NewPauseCampaignHandler builds the handler, failing fast on a nil dependency.
func NewPauseCampaignHandler(campaigns domain.CampaignRepository) PauseCampaignHandler {
	if campaigns == nil {
		panic("nil campaign repository")
	}
	return PauseCampaignHandler{campaigns: campaigns}
}

// Handle pauses a running campaign.
func (h PauseCampaignHandler) Handle(ctx context.Context, cmd PauseCampaign) error {
	return h.campaigns.Update(ctx, cmd.TenantID, cmd.CampaignID,
		func(c *domain.Campaign) (*domain.Campaign, error) {
			return c, c.Pause()
		})
}

// CancelCampaign is the request to abandon a campaign before it finishes.
type CancelCampaign struct {
	TenantID   string
	CampaignID string
}

// CancelCampaignHandler handles the CancelCampaign command.
type CancelCampaignHandler struct {
	campaigns domain.CampaignRepository
}

// NewCancelCampaignHandler builds the handler, failing fast on a nil dependency.
func NewCancelCampaignHandler(campaigns domain.CampaignRepository) CancelCampaignHandler {
	if campaigns == nil {
		panic("nil campaign repository")
	}
	return CancelCampaignHandler{campaigns: campaigns}
}

// Handle cancels a campaign that has not yet finished.
func (h CancelCampaignHandler) Handle(ctx context.Context, cmd CancelCampaign) error {
	return h.campaigns.Update(ctx, cmd.TenantID, cmd.CampaignID,
		func(c *domain.Campaign) (*domain.Campaign, error) {
			return c, c.Cancel()
		})
}

// ResumeCampaign is the request to resume a paused campaign.
type ResumeCampaign struct {
	TenantID   string
	CampaignID string
}

// ResumeCampaignHandler handles the ResumeCampaign command.
type ResumeCampaignHandler struct {
	campaigns domain.CampaignRepository
	enqueuer  CampaignEnqueuer
}

// NewResumeCampaignHandler builds the handler, failing fast on a nil dependency.
func NewResumeCampaignHandler(campaigns domain.CampaignRepository,
	enqueuer CampaignEnqueuer) ResumeCampaignHandler {
	if campaigns == nil || enqueuer == nil {
		panic("nil dependency")
	}
	return ResumeCampaignHandler{campaigns: campaigns, enqueuer: enqueuer}
}

// Handle resumes a paused campaign and re-enqueues its send so the still-
// pending recipients are picked up.
func (h ResumeCampaignHandler) Handle(ctx context.Context, cmd ResumeCampaign) error {
	if err := h.campaigns.Update(ctx, cmd.TenantID, cmd.CampaignID,
		func(c *domain.Campaign) (*domain.Campaign, error) {
			return c, c.Resume()
		}); err != nil {
		return err
	}
	return h.enqueuer.EnqueueStart(ctx, cmd.TenantID, cmd.CampaignID)
}

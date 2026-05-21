package command

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/adapters/visualrender"
	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// FieldsProvider exposes the consolidated set of subscriber-field slugs
// (built-in pseudo-rows + tenant-defined custom fields) for a tenant. The
// save_visual_{campaign,template} commands wrap the returned slug set as a
// domain.FieldSet for save-time placeholder validation (FR-016c).
//
// Declared here, by the command layer that depends on it, and implemented in
// the composition root (internal/service) over the audience FieldRepository.
type FieldsProvider interface {
	AllSlugs(ctx context.Context, tenantID string) (map[string]bool, error)
}

// MediaRefValidator validates that an image-block MediaRef points at a
// tenant-scoped media-library URL (FR-021). It is the consumer side of the
// domain.MediaRefValidator interface and is wired in the composition root
// against the platform's media base URL config.
type MediaRefValidator interface {
	IsTenantMediaRef(ref string) bool
}

// SaveVisualCampaign is the command issued by the BFF when an operator
// saves a visually-authored campaign. The BFF has already rendered the doc
// to HTML and plain text via @react-email/components; this command's
// responsibility is to re-validate the doc against the registry (defense
// in depth), sanitize the rendered HTML, enforce the optimistic-concurrency
// gate from FR-009, and persist all three pieces atomically.
//
// Doc / PinnedTheme are the typed forms used for save-time validation.
// DocJSON / ThemeJSON are the raw wire bytes the BFF sent — they reach
// the row verbatim so the editor reloads losslessly.
type SaveVisualCampaign struct {
	TenantID          string
	CampaignID        string
	Subject           string
	Doc               *domain.VisualDoc
	BodyHTML          string
	BodyText          string
	PinnedTheme       *domain.Theme
	DocJSON           json.RawMessage
	ThemeJSON         json.RawMessage
	IfUnmodifiedSince time.Time
}

// SaveVisualCampaignResult carries the post-save outcome the handler
// surfaces to the API. UpdatedAt is the row's new updated_at after the
// successful write; on ErrStaleRow the row was not changed and
// CurrentUpdatedAt holds the value the SPA must adopt before retrying.
type SaveVisualCampaignResult struct {
	UpdatedAt        time.Time
	Warnings         []domain.RenderWarning
	CurrentUpdatedAt time.Time
}

// SaveVisualCampaignHandler handles the SaveVisualCampaign command.
type SaveVisualCampaignHandler struct {
	campaigns domain.CampaignRepository
	fields    FieldsProvider
	mediaRefs MediaRefValidator
}

// NewSaveVisualCampaignHandler builds the handler, failing fast on a nil
// dependency. The repository owns the tenant-bound transaction; the
// fields/mediaRefs providers are pure read-side and called outside the write
// transaction.
func NewSaveVisualCampaignHandler(campaigns domain.CampaignRepository,
	fields FieldsProvider, mediaRefs MediaRefValidator) SaveVisualCampaignHandler {
	if campaigns == nil || fields == nil || mediaRefs == nil {
		panic("nil dependency")
	}
	return SaveVisualCampaignHandler{campaigns: campaigns, fields: fields, mediaRefs: mediaRefs}
}

// slugSetFieldSet adapts a slug→true map to domain.FieldSet without
// allocating per-call wrapper structs at the call site.
type slugSetFieldSet map[string]bool

func (s slugSetFieldSet) HasSlug(slug string) bool { return s[slug] }

// Handle runs the save: resolve the field set, sanitize the BFF-rendered
// HTML, then inside the write transaction check the FR-009 stale-row gate
// and mutate the aggregate atomically.
func (h SaveVisualCampaignHandler) Handle(ctx context.Context,
	cmd SaveVisualCampaign) (SaveVisualCampaignResult, error) {
	var res SaveVisualCampaignResult

	if cmd.IfUnmodifiedSince.IsZero() {
		return res, domain.ErrCampaignInvalid.WithMessage("ifUnmodifiedSince is required")
	}
	if strings.TrimSpace(cmd.BodyHTML) == "" || strings.TrimSpace(cmd.BodyText) == "" {
		return res, domain.ErrCampaignInvalid.WithMessage("bodyHtml and bodyText are required")
	}

	slugs, err := h.fields.AllSlugs(ctx, cmd.TenantID)
	if err != nil {
		return res, err
	}
	sanitizedHTML, warnings := visualrender.Sanitize(cmd.BodyHTML)

	updateErr := h.campaigns.Update(ctx, cmd.TenantID, cmd.CampaignID,
		func(c *domain.Campaign) (*domain.Campaign, error) {
			if !cmd.IfUnmodifiedSince.Equal(c.UpdatedAt()) {
				res.CurrentUpdatedAt = c.UpdatedAt()
				return nil, domain.ErrStaleRow
			}
			if err := c.ApplyVisualSave(
				cmd.Subject, cmd.Doc, cmd.PinnedTheme,
				sanitizedHTML, cmd.BodyText,
				cmd.DocJSON, cmd.ThemeJSON, warnings,
				slugSetFieldSet(slugs), h.mediaRefs,
			); err != nil {
				return nil, err
			}
			return c, nil
		})
	if updateErr != nil {
		if errors.Is(updateErr, domain.ErrStaleRow) {
			return res, updateErr
		}
		return SaveVisualCampaignResult{}, updateErr
	}

	after, err := h.campaigns.Get(ctx, cmd.TenantID, cmd.CampaignID)
	if err != nil {
		return res, err
	}
	res.UpdatedAt = after.UpdatedAt()
	res.Warnings = warnings
	return res, nil
}

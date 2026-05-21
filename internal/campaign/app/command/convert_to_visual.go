package command

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/adapters/visualrender"
	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// ConvertCampaignToVisual converts an existing campaign's body_html into a
// candidate VisualDoc the operator can open in the visual editor for review.
// The conversion is non-persisting (per contracts/tenant-api.md
// "convert-to-visual"); the operator saves through the regular visual PUT
// after editing the candidate doc.
type ConvertCampaignToVisual struct {
	TenantID   string
	CampaignID string
}

// ConvertedDocResult is the candidate doc plus its conversion warnings.
type ConvertedDocResult struct {
	// BodyDoc carries the JSON-encoded VisualDoc the converter produced. The
	// handler encodes the typed doc here so the HTTP layer can forward it
	// verbatim without depending on the BFF's encoder.
	BodyDoc json.RawMessage
	// Warnings lists every RawHTML fallback the converter emitted, paired
	// with the path of the resulting block in the converted doc.
	Warnings []visualrender.ConversionWarning
}

// ConvertCampaignToVisualHandler handles the ConvertCampaignToVisual command.
type ConvertCampaignToVisualHandler struct {
	campaigns domain.CampaignRepository
	mediaRefs MediaRefValidator
}

// NewConvertCampaignToVisualHandler builds the handler, failing fast on a nil
// dependency. mediaRefs may be nil — the converter then accepts every image
// src and the validator will reject non-tenant refs at save time instead.
func NewConvertCampaignToVisualHandler(campaigns domain.CampaignRepository,
	mediaRefs MediaRefValidator) ConvertCampaignToVisualHandler {
	if campaigns == nil {
		panic("nil campaign repository")
	}
	return ConvertCampaignToVisualHandler{campaigns: campaigns, mediaRefs: mediaRefs}
}

// Handle loads the campaign, refuses if it already carries a visual document
// (per the "already_visual" contract), and otherwise runs the converter over
// its persisted body_html.
func (h ConvertCampaignToVisualHandler) Handle(ctx context.Context,
	cmd ConvertCampaignToVisual) (ConvertedDocResult, error) {
	c, err := h.campaigns.Get(ctx, cmd.TenantID, cmd.CampaignID)
	if err != nil {
		return ConvertedDocResult{}, err
	}
	if c.BodyDocJSON() != nil {
		return ConvertedDocResult{}, domain.ErrAlreadyVisual
	}
	body := c.BodyHTML()
	if strings.TrimSpace(body) == "" {
		return ConvertedDocResult{}, domain.ErrVisualDocInvalid.WithMessage("campaign has no HTML body to convert")
	}
	return convertWithValidator(body, h.mediaRefs)
}

// ConvertTemplateToVisual is the templates counterpart of
// ConvertCampaignToVisual.
type ConvertTemplateToVisual struct {
	TenantID   string
	TemplateID string
}

// ConvertTemplateToVisualHandler handles the ConvertTemplateToVisual command.
type ConvertTemplateToVisualHandler struct {
	templates domain.TemplateRepository
	mediaRefs MediaRefValidator
}

// NewConvertTemplateToVisualHandler builds the handler, failing fast on a nil
// dependency.
func NewConvertTemplateToVisualHandler(templates domain.TemplateRepository,
	mediaRefs MediaRefValidator) ConvertTemplateToVisualHandler {
	if templates == nil {
		panic("nil template repository")
	}
	return ConvertTemplateToVisualHandler{templates: templates, mediaRefs: mediaRefs}
}

// Handle loads the template, refuses if it already carries a visual document,
// and otherwise runs the converter over its persisted body_html.
func (h ConvertTemplateToVisualHandler) Handle(ctx context.Context,
	cmd ConvertTemplateToVisual) (ConvertedDocResult, error) {
	t, err := h.templates.Get(ctx, cmd.TenantID, cmd.TemplateID)
	if err != nil {
		return ConvertedDocResult{}, err
	}
	if t.BodyDocJSON() != nil {
		return ConvertedDocResult{}, domain.ErrAlreadyVisual
	}
	body := t.BodyHTML()
	if strings.TrimSpace(body) == "" {
		return ConvertedDocResult{}, domain.ErrVisualDocInvalid.WithMessage("template has no HTML body to convert")
	}
	return convertWithValidator(body, h.mediaRefs)
}

// mediaRefAdapter bridges command.MediaRefValidator (an interface declared by
// the command layer next to the save handlers) to the converter's
// domain.MediaRefValidator interface — same shape, different package.
type mediaRefAdapter struct{ inner MediaRefValidator }

func (a mediaRefAdapter) IsTenantMediaRef(ref string) bool {
	return a.inner != nil && a.inner.IsTenantMediaRef(ref)
}

// convertWithValidator runs the converter and encodes the resulting doc as
// JSON for the HTTP wire shape. Used by both the campaign and template
// handlers so the wire shape stays uniform.
func convertWithValidator(body string, mediaRefs MediaRefValidator) (ConvertedDocResult, error) {
	var opts visualrender.ConvertOptions
	if mediaRefs != nil {
		opts.MediaRefs = mediaRefAdapter{inner: mediaRefs}
	}
	doc, warnings, err := visualrender.Convert(body, opts)
	if err != nil {
		return ConvertedDocResult{}, err
	}
	encoded, err := domain.MarshalVisualDoc(doc)
	if err != nil {
		return ConvertedDocResult{}, err
	}
	if warnings == nil {
		warnings = []visualrender.ConversionWarning{}
	}
	return ConvertedDocResult{BodyDoc: encoded, Warnings: warnings}, nil
}

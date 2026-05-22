package command

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/campaign/visualrender"
)

// SaveVisualTemplate is the command issued by the BFF when an operator
// saves a visually-authored template (T072). The BFF has already
// rendered the doc to HTML and plain text via @react-email/components;
// this command's responsibility is to re-validate the doc against the
// registry (defense in depth), sanitize the rendered HTML, enforce the
// FR-009 optimistic-concurrency gate, and persist all three pieces
// atomically — the same flow as SaveVisualCampaign.
//
// Doc / PinnedTheme are the typed forms used for save-time validation.
// DocJSON / ThemeJSON are the raw wire bytes the BFF sent — they reach
// the row verbatim so the editor reloads losslessly.
type SaveVisualTemplate struct {
	TenantID          string
	TemplateID        string
	Subject           string
	Doc               *domain.VisualDoc
	BodyHTML          string
	BodyText          string
	PinnedTheme       *domain.Theme
	DocJSON           json.RawMessage
	ThemeJSON         json.RawMessage
	IfUnmodifiedSince time.Time
}

// SaveVisualTemplateResult carries the post-save outcome the handler
// surfaces to the API. UpdatedAt is the row's new updated_at after the
// successful write; on ErrStaleRow the row was not changed and
// CurrentUpdatedAt holds the value the SPA must adopt before retrying.
type SaveVisualTemplateResult struct {
	UpdatedAt        time.Time
	Warnings         []domain.RenderWarning
	CurrentUpdatedAt time.Time
}

// SaveVisualTemplateHandler handles the SaveVisualTemplate command.
type SaveVisualTemplateHandler struct {
	templates domain.TemplateRepository
	fields    FieldsProvider
	mediaRefs MediaRefValidator
}

// NewSaveVisualTemplateHandler builds the handler, failing fast on a nil
// dependency. The repository owns the tenant-bound transaction; the
// fields/mediaRefs providers are pure read-side and called outside the
// write transaction.
func NewSaveVisualTemplateHandler(templates domain.TemplateRepository,
	fields FieldsProvider, mediaRefs MediaRefValidator) SaveVisualTemplateHandler {
	if templates == nil || fields == nil || mediaRefs == nil {
		panic("nil dependency")
	}
	return SaveVisualTemplateHandler{templates: templates, fields: fields, mediaRefs: mediaRefs}
}

// Handle runs the save: resolve the field set, sanitize the BFF-rendered
// HTML, then inside the write transaction check the FR-009 stale-row gate
// and mutate the aggregate atomically.
func (h SaveVisualTemplateHandler) Handle(ctx context.Context,
	cmd SaveVisualTemplate) (SaveVisualTemplateResult, error) {
	var res SaveVisualTemplateResult

	if cmd.IfUnmodifiedSince.IsZero() {
		return res, domain.ErrTemplateInvalid.WithMessage("ifUnmodifiedSince is required")
	}
	if strings.TrimSpace(cmd.BodyHTML) == "" || strings.TrimSpace(cmd.BodyText) == "" {
		return res, domain.ErrTemplateInvalid.WithMessage("bodyHtml and bodyText are required")
	}

	slugs, err := h.fields.AllSlugs(ctx, cmd.TenantID)
	if err != nil {
		return res, err
	}
	sanitizedHTML, warnings := visualrender.Sanitize(cmd.BodyHTML)

	updateErr := h.templates.Update(ctx, cmd.TenantID, cmd.TemplateID,
		func(t *domain.Template) (*domain.Template, error) {
			if !cmd.IfUnmodifiedSince.Equal(t.UpdatedAt()) {
				res.CurrentUpdatedAt = t.UpdatedAt()
				return nil, domain.ErrStaleRow
			}
			if err := t.ApplyVisualSave(
				cmd.Subject, cmd.Doc, cmd.PinnedTheme,
				sanitizedHTML, cmd.BodyText,
				cmd.DocJSON, cmd.ThemeJSON, warnings,
				slugSetFieldSet(slugs), h.mediaRefs,
			); err != nil {
				return nil, err
			}
			return t, nil
		})
	if updateErr != nil {
		if errors.Is(updateErr, domain.ErrStaleRow) {
			return res, updateErr
		}
		return SaveVisualTemplateResult{}, updateErr
	}

	after, err := h.templates.Get(ctx, cmd.TenantID, cmd.TemplateID)
	if err != nil {
		return res, err
	}
	res.UpdatedAt = after.UpdatedAt()
	res.Warnings = warnings
	return res, nil
}

package domain

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"
)

// CampaignStatus is the lifecycle state of a campaign.
type CampaignStatus string

const (
	// CampaignDraft is an unsent campaign still being authored.
	CampaignDraft CampaignStatus = "draft"
	// CampaignRunning is a campaign actively sending.
	CampaignRunning CampaignStatus = "running"
	// CampaignPaused is a campaign whose send is suspended and resumable.
	CampaignPaused CampaignStatus = "paused"
	// CampaignFinished is a campaign whose every recipient has been processed.
	CampaignFinished CampaignStatus = "finished"
	// CampaignCancelled is a campaign abandoned before completion.
	CampaignCancelled CampaignStatus = "cancelled"
)

// localPartPattern matches a valid email address local part.
var localPartPattern = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+$`)

// Campaign is a tenant-plane aggregate: an authored message progressing
// through a draft → running → finished lifecycle, carrying its send progress.
// It is reached only through the RLS-bound transaction owned by its repository
// adapter.
//
// A campaign may also carry a structured visual document and an explicit
// theme override (populated by NewVisualCampaign). When bodyDoc is nil the
// row is either a legacy raw-HTML campaign or one the operator opted out of
// the visual editor on. When theme is nil the renderer inherits the tenant's
// Phase 6 branding defaults at render time.
type Campaign struct {
	id              string
	tenantID        string
	name            string
	subject         string
	bodyHTML        string
	bodyText        string
	bodyDoc         *VisualDoc
	theme           *Theme
	bodyDocJSON     json.RawMessage
	themeJSON       json.RawMessage
	warnings        []RenderWarning
	fromName        string
	fromLocalPart   string
	sendingDomainID string
	templateID      string
	status          CampaignStatus
	maxSendErrors   int
	sentCount       int
	failedCount     int
	recipientCount  int
	createdAt       time.Time
	updatedAt       time.Time
	startedAt       *time.Time
	finishedAt      *time.Time
	archiveVisible  bool
	archivedAt      *time.Time
}

// NewCampaign builds a draft campaign, rejecting any invariant violation. A
// draft only needs a name; its content (subject, body, From address, sending
// domain) may be filled in later and is enforced by Start.
func NewCampaign(tenantID, name, subject, bodyHTML, bodyText, fromName, fromLocalPart,
	sendingDomainID, templateID string, maxSendErrors int) (*Campaign, error) {

	if tenantID == "" {
		return nil, ErrCampaignInvalid.WithMessage("a tenant is required")
	}
	name = strings.TrimSpace(name)
	subject = strings.TrimSpace(subject)
	fromLocalPart = strings.TrimSpace(fromLocalPart)
	if name == "" {
		return nil, ErrCampaignInvalid.WithMessage("campaign name is required")
	}
	if fromLocalPart != "" && !localPartPattern.MatchString(fromLocalPart) {
		return nil, ErrCampaignInvalid.WithMessage("the From address local part is not valid")
	}
	if maxSendErrors <= 0 {
		maxSendErrors = 100
	}
	return &Campaign{
		tenantID: tenantID, name: name, subject: subject,
		bodyHTML: bodyHTML, bodyText: bodyText,
		fromName: strings.TrimSpace(fromName), fromLocalPart: fromLocalPart,
		sendingDomainID: sendingDomainID, templateID: templateID,
		status: CampaignDraft, maxSendErrors: maxSendErrors,
	}, nil
}

// HydrateCampaign reconstructs a campaign from a persisted row. Persistence
// only — it performs no validation. bodyDocJSON and themeJSON are the raw
// jsonb column bytes (nil for legacy raw-HTML rows that predate Phase 7 or
// for rows the operator opted out of the visual editor on); the typed
// VisualDoc / Theme pointers are not reconstructed from the bytes — the
// editor reloads from the JSON via the query view, and save-time validation
// runs against the freshly-decoded BFF payload.
func HydrateCampaign(id, tenantID, name, subject, bodyHTML, bodyText, fromName, fromLocalPart,
	sendingDomainID, templateID string, status CampaignStatus,
	maxSendErrors, sentCount, failedCount, recipientCount int,
	bodyDocJSON, themeJSON json.RawMessage,
	createdAt, updatedAt time.Time, startedAt, finishedAt *time.Time,
	archiveVisible bool, archivedAt *time.Time) *Campaign {

	return &Campaign{
		id: id, tenantID: tenantID, name: name, subject: subject,
		bodyHTML: bodyHTML, bodyText: bodyText,
		bodyDocJSON: normalizeRawJSON(bodyDocJSON),
		themeJSON:   normalizeRawJSON(themeJSON),
		fromName:    fromName, fromLocalPart: fromLocalPart,
		sendingDomainID: sendingDomainID, templateID: templateID,
		status: status, maxSendErrors: maxSendErrors,
		sentCount: sentCount, failedCount: failedCount, recipientCount: recipientCount,
		createdAt: createdAt, updatedAt: updatedAt,
		startedAt: startedAt, finishedAt: finishedAt,
		archiveVisible: archiveVisible, archivedAt: archivedAt,
	}
}

// NewVisualCampaign builds a draft campaign authored visually. The caller
// (the save_visual_campaign command) is responsible for rendering the doc
// to HTML and plain text — typically by side-calling the BFF render tier
// (see specs/014-visual-email-editor/research.md § R4) and then running
// the BFF-rendered HTML through the Go-side bluemonday sanitizer
// (internal/campaign/adapters/visualrender.Sanitize). This constructor
// revalidates the doc against the registry and media-ref rules as
// defense in depth and returns the populated aggregate with all three
// pieces (body_doc, body_html, body_text) atomically.
//
// docJSON and themeJSON are the raw wire bytes the BFF sent — persisted
// pass-through so the editor reloads losslessly from the JSON form.
//
// pinnedTheme is the operator's explicit override and may be nil; the row
// then persists a NULL theme and inherits tenant branding at future
// render time. warnings are the sanitizer-emitted notes from the
// caller's sanitization pass; the aggregate carries them so the handler
// can return them to the operator in the save response.
func NewVisualCampaign(
	tenantID, name, subject string,
	doc *VisualDoc, pinnedTheme *Theme,
	bodyHTML, bodyText string,
	docJSON, themeJSON json.RawMessage,
	warnings []RenderWarning,
	fromName, fromLocalPart, sendingDomainID, templateID string, maxSendErrors int,
	fields FieldSet, mediaRefs MediaRefValidator,
) (*Campaign, error) {
	if tenantID == "" {
		return nil, ErrCampaignInvalid.WithMessage("a tenant is required")
	}
	name = strings.TrimSpace(name)
	subject = strings.TrimSpace(subject)
	fromLocalPart = strings.TrimSpace(fromLocalPart)
	if name == "" {
		return nil, ErrCampaignInvalid.WithMessage("campaign name is required")
	}
	if fromLocalPart != "" && !localPartPattern.MatchString(fromLocalPart) {
		return nil, ErrCampaignInvalid.WithMessage("the From address local part is not valid")
	}
	if doc == nil {
		return nil, ErrVisualDocInvalid.WithMessage("document is required")
	}
	if err := Validate(doc, ValidateContext{Fields: fields, MediaRefs: mediaRefs}); err != nil {
		return nil, err
	}
	if maxSendErrors <= 0 {
		maxSendErrors = 100
	}
	return &Campaign{
		tenantID: tenantID, name: name, subject: subject,
		bodyHTML: bodyHTML, bodyText: bodyText,
		bodyDoc: doc, theme: pinnedTheme,
		bodyDocJSON: normalizeRawJSON(docJSON),
		themeJSON:   normalizeRawJSON(themeJSON),
		warnings:    warnings,
		fromName:    strings.TrimSpace(fromName), fromLocalPart: fromLocalPart,
		sendingDomainID: sendingDomainID, templateID: templateID,
		status: CampaignDraft, maxSendErrors: maxSendErrors,
	}, nil
}

// ID returns the database-assigned id.
func (c *Campaign) ID() string { return c.id }

// TenantID returns the owning tenant's id.
func (c *Campaign) TenantID() string { return c.tenantID }

// Name returns the campaign name.
func (c *Campaign) Name() string { return c.name }

// Subject returns the subject line.
func (c *Campaign) Subject() string { return c.subject }

// BodyHTML returns the HTML body.
func (c *Campaign) BodyHTML() string { return c.bodyHTML }

// BodyText returns the plain-text body.
func (c *Campaign) BodyText() string { return c.bodyText }

// BodyDoc returns the structured visual document, or nil for legacy
// raw-HTML / code-only campaigns.
func (c *Campaign) BodyDoc() *VisualDoc { return c.bodyDoc }

// BodyDocJSON returns the persisted JSON bytes of the visual document, or
// nil for legacy raw-HTML / code-only campaigns. The bytes are the same
// shape the BFF sent on the most recent visual save — the read view
// passes them through verbatim so the editor reloads losslessly.
func (c *Campaign) BodyDocJSON() json.RawMessage { return c.bodyDocJSON }

// Theme returns the explicit theme override, or nil when the row inherits
// tenant Phase 6 branding defaults at render time.
func (c *Campaign) Theme() *Theme { return c.theme }

// ThemeJSON returns the persisted JSON bytes of the operator's pinned
// theme override, or nil when the row inherits tenant branding defaults.
func (c *Campaign) ThemeJSON() json.RawMessage { return c.themeJSON }

// AttachVisualContent copies a template's pre-rendered visual content onto
// this campaign at create-from-template time. Persistence pass-through —
// the bytes reach the row on Add(). Used by CreateCampaign to inherit the
// origin template's body_doc + theme alongside the existing subject /
// body_html / body_text inheritance (per T076).
func (c *Campaign) AttachVisualContent(bodyDocJSON, themeJSON json.RawMessage) {
	c.bodyDocJSON = normalizeRawJSON(bodyDocJSON)
	c.themeJSON = normalizeRawJSON(themeJSON)
}

// RenderWarnings returns the non-fatal warnings emitted by the most recent
// NewVisualCampaign construction. Empty for hydrated rows.
func (c *Campaign) RenderWarnings() []RenderWarning { return c.warnings }

// FromName returns the sender display name.
func (c *Campaign) FromName() string { return c.fromName }

// FromLocalPart returns the local part of the From address.
func (c *Campaign) FromLocalPart() string { return c.fromLocalPart }

// SendingDomainID returns the selected sending domain's id, or "".
func (c *Campaign) SendingDomainID() string { return c.sendingDomainID }

// TemplateID returns the origin template's id, or "".
func (c *Campaign) TemplateID() string { return c.templateID }

// Status returns the lifecycle status.
func (c *Campaign) Status() CampaignStatus { return c.status }

// MaxSendErrors returns the auto-pause threshold.
func (c *Campaign) MaxSendErrors() int { return c.maxSendErrors }

// SentCount returns how many recipients have been sent to.
func (c *Campaign) SentCount() int { return c.sentCount }

// FailedCount returns how many recipient sends have failed.
func (c *Campaign) FailedCount() int { return c.failedCount }

// RecipientCount returns the total resolved recipient count.
func (c *Campaign) RecipientCount() int { return c.recipientCount }

// CreatedAt returns when the campaign was created.
func (c *Campaign) CreatedAt() time.Time { return c.createdAt }

// UpdatedAt returns when the campaign was last changed.
func (c *Campaign) UpdatedAt() time.Time { return c.updatedAt }

// StartedAt returns when the campaign send began, or nil.
func (c *Campaign) StartedAt() *time.Time { return c.startedAt }

// FinishedAt returns when the campaign send completed, or nil.
func (c *Campaign) FinishedAt() *time.Time { return c.finishedAt }

// IsDraft reports whether the campaign is still an editable draft.
func (c *Campaign) IsDraft() bool { return c.status == CampaignDraft }

// IsRunning reports whether the campaign is actively sending.
func (c *Campaign) IsRunning() bool { return c.status == CampaignRunning }

// Recompose replaces the campaign's editable content. It is rejected unless
// the campaign is still a draft.
func (c *Campaign) Recompose(name, subject, bodyHTML, bodyText, fromName, fromLocalPart,
	sendingDomainID string) error {
	if c.status != CampaignDraft {
		return ErrCampaignNotEditable
	}
	updated, err := NewCampaign(c.tenantID, name, subject, bodyHTML, bodyText,
		fromName, fromLocalPart, sendingDomainID, c.templateID, c.maxSendErrors)
	if err != nil {
		return err
	}
	c.name = updated.name
	c.subject = updated.subject
	c.bodyHTML = updated.bodyHTML
	c.bodyText = updated.bodyText
	c.fromName = updated.fromName
	c.fromLocalPart = updated.fromLocalPart
	c.sendingDomainID = sendingDomainID
	return nil
}

// OptOutVisual clears the campaign's structured visual document and theme
// override, leaving body_html / body_text intact so the campaign remains
// sendable as a code-only campaign (per FR-029 / contracts/tenant-api.md
// "opt-out-visual"). Only a draft campaign may be edited; the method is a
// no-op rejection (ErrCampaignNotEditable) otherwise. Idempotent: calling
// it on a row that already has no body_doc is a no-op success.
func (c *Campaign) OptOutVisual() error {
	if c.status != CampaignDraft {
		return ErrCampaignNotEditable
	}
	c.bodyDoc = nil
	c.theme = nil
	c.bodyDocJSON = nil
	c.themeJSON = nil
	c.warnings = nil
	return nil
}

// ApplyVisualSave replaces a draft campaign's editable visual content with a
// validated, pre-rendered, and sanitized snapshot. The caller (the
// SaveVisualCampaign command) supplies the BFF-rendered HTML/text that
// has already been run through the Go-side sanitizer, plus any warnings
// the sanitizer emitted; this method revalidates the doc against the
// registry and media-ref rules as defense in depth and applies all
// pieces atomically.
//
// docJSON and themeJSON are the raw wire bytes the BFF sent — persisted
// pass-through so the editor reloads losslessly.
//
// Only a draft campaign may be edited; the method is a no-op rejection
// (ErrCampaignNotEditable) otherwise. Subject is required; the campaign's
// name, From-address, sending domain, and targets are preserved.
func (c *Campaign) ApplyVisualSave(
	subject string, doc *VisualDoc, pinnedTheme *Theme,
	bodyHTML, bodyText string,
	docJSON, themeJSON json.RawMessage,
	warnings []RenderWarning,
	fields FieldSet, mediaRefs MediaRefValidator,
) error {
	if c.status != CampaignDraft {
		return ErrCampaignNotEditable
	}
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return ErrCampaignInvalid.WithMessage("campaign subject is required")
	}
	if doc == nil {
		return ErrVisualDocInvalid.WithMessage("document is required")
	}
	if err := Validate(doc, ValidateContext{Fields: fields, MediaRefs: mediaRefs}); err != nil {
		return err
	}
	c.subject = subject
	c.bodyDoc = doc
	c.theme = pinnedTheme
	c.bodyHTML = bodyHTML
	c.bodyText = bodyText
	c.bodyDocJSON = normalizeRawJSON(docJSON)
	c.themeJSON = normalizeRawJSON(themeJSON)
	c.warnings = warnings
	return nil
}

// Start transitions a draft campaign to running. It requires the campaign's
// content to be complete and a selected sending domain; the caller is
// responsible for confirming the domain is verified and that targets exist.
func (c *Campaign) Start(at time.Time) error {
	if c.status != CampaignDraft {
		return ErrCampaignNotDraft
	}
	if c.subject == "" {
		return ErrCampaignInvalid.WithMessage("campaign subject is required")
	}
	if strings.TrimSpace(c.bodyHTML) == "" && strings.TrimSpace(c.bodyText) == "" {
		return ErrCampaignInvalid.WithMessage("a campaign needs an HTML or text body")
	}
	if c.fromLocalPart == "" {
		return ErrCampaignInvalid.WithMessage("a valid From address local part is required")
	}
	if c.sendingDomainID == "" {
		return ErrSendingDomainRequired
	}
	c.status = CampaignRunning
	at = at.UTC()
	c.startedAt = &at
	return nil
}

// Pause suspends a running campaign. The pause is recoverable.
func (c *Campaign) Pause() error {
	if c.status != CampaignRunning {
		return ErrCampaignNotEditable.WithMessage("only a running campaign can be paused")
	}
	c.status = CampaignPaused
	return nil
}

// Resume returns a paused campaign to running.
func (c *Campaign) Resume() error {
	if c.status != CampaignPaused {
		return ErrCampaignNotEditable.WithMessage("only a paused campaign can be resumed")
	}
	c.status = CampaignRunning
	return nil
}

// Finish marks a running campaign complete once no recipients remain.
func (c *Campaign) Finish(at time.Time) error {
	if c.status != CampaignRunning {
		return ErrCampaignNotEditable.WithMessage("only a running campaign can finish")
	}
	c.status = CampaignFinished
	at = at.UTC()
	c.finishedAt = &at
	return nil
}

// Cancel abandons a campaign that has not yet finished.
func (c *Campaign) Cancel() error {
	switch c.status {
	case CampaignDraft, CampaignRunning, CampaignPaused:
		c.status = CampaignCancelled
		return nil
	default:
		return ErrCampaignNotEditable.WithMessage("a finished or cancelled campaign cannot be cancelled")
	}
}

// SetRecipientCount records the total resolved recipient count, set once the
// start worker has materialised the recipient set.
func (c *Campaign) SetRecipientCount(n int) { c.recipientCount = n }

// RecordProgress adds to the sent and failed counters.
func (c *Campaign) RecordProgress(sent, failed int) {
	c.sentCount += sent
	c.failedCount += failed
}

// SyncProgress sets the sent and failed counters to absolute values — used by
// the batch worker, which derives them from the per-recipient rows so a
// redelivered job never double-counts.
func (c *Campaign) SyncProgress(sent, failed int) {
	c.sentCount = sent
	c.failedCount = failed
}

// ShouldAutoPause reports whether accumulated send failures have crossed the
// campaign's threshold while it is still running.
func (c *Campaign) ShouldAutoPause() bool {
	return c.status == CampaignRunning && c.failedCount > c.maxSendErrors
}

// ArchiveVisible reports whether the campaign is exposed on the tenant's
// public archive and RSS feed.
func (c *Campaign) ArchiveVisible() bool { return c.archiveVisible }

// ArchivedAt returns when the campaign was first made archive-visible, or nil.
func (c *Campaign) ArchivedAt() *time.Time { return c.archivedAt }

// SetArchiveVisible toggles a campaign's archive visibility. Only a campaign
// whose send has begun may be made archive-visible — a draft or never-sent
// campaign has no audience-facing content yet. Idempotent: setting the same
// value preserves the original archived_at timestamp.
func (c *Campaign) SetArchiveVisible(v bool, at time.Time) error {
	if v && c.startedAt == nil {
		return ErrCampaignNotSent
	}
	if v == c.archiveVisible {
		return nil
	}
	c.archiveVisible = v
	if v && c.archivedAt == nil {
		at = at.UTC()
		c.archivedAt = &at
	}
	return nil
}

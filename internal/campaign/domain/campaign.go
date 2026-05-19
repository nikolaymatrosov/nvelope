package domain

import (
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
type Campaign struct {
	id              string
	tenantID        string
	name            string
	subject         string
	bodyHTML        string
	bodyText        string
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
// only — it performs no validation.
func HydrateCampaign(id, tenantID, name, subject, bodyHTML, bodyText, fromName, fromLocalPart,
	sendingDomainID, templateID string, status CampaignStatus,
	maxSendErrors, sentCount, failedCount, recipientCount int,
	createdAt, updatedAt time.Time, startedAt, finishedAt *time.Time) *Campaign {

	return &Campaign{
		id: id, tenantID: tenantID, name: name, subject: subject,
		bodyHTML: bodyHTML, bodyText: bodyText,
		fromName: fromName, fromLocalPart: fromLocalPart,
		sendingDomainID: sendingDomainID, templateID: templateID,
		status: status, maxSendErrors: maxSendErrors,
		sentCount: sentCount, failedCount: failedCount, recipientCount: recipientCount,
		createdAt: createdAt, updatedAt: updatedAt,
		startedAt: startedAt, finishedAt: finishedAt,
	}
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

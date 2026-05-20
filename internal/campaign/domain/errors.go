// Package domain holds the campaign bounded context's business types:
// templates, campaigns, per-recipient send progress, and link/open tracking.
// It imports nothing from the app, adapters, or transport layers.
package domain

import "github.com/nikolaymatrosov/nvelope/internal/platform/apperr"

// Typed campaign-domain errors. Each carries the stable response slug and the
// transport-agnostic category; internal/api/errmap.go maps the category (and a
// few slug overrides) to an HTTP status in one place.
var (
	// ErrTemplateNameTaken is returned when a template name is already used.
	ErrTemplateNameTaken = apperr.NewConflict("template-name-taken",
		"a template with that name already exists")

	// ErrTemplateInvalid is returned when template content fails validation.
	ErrTemplateInvalid = apperr.NewIncorrectInput("template-invalid",
		"the template is missing required content")

	// ErrTemplateNotFound is returned when no template matches a lookup.
	ErrTemplateNotFound = apperr.NewNotFound("template-not-found", "no such template")

	// ErrTemplateKindMismatch is returned when a template of the wrong kind is
	// used — a campaign built from a transactional template, or vice versa.
	ErrTemplateKindMismatch = apperr.NewIncorrectInput("template-kind-mismatch",
		"that template cannot be used here")

	// ErrCampaignInvalid is returned when campaign content fails validation.
	ErrCampaignInvalid = apperr.NewIncorrectInput("campaign-invalid",
		"the campaign is missing required content")

	// ErrCampaignNotFound is returned when no campaign matches a lookup.
	ErrCampaignNotFound = apperr.NewNotFound("campaign-not-found", "no such campaign")

	// ErrCampaignNotDraft is returned when a start is attempted on a campaign
	// that is not in draft.
	ErrCampaignNotDraft = apperr.NewConflict("campaign-not-draft",
		"only a draft campaign can be started")

	// ErrCampaignNotEditable is returned when editing a campaign that is no
	// longer a draft.
	ErrCampaignNotEditable = apperr.NewConflict("campaign-not-editable",
		"only a draft campaign can be edited")

	// ErrSendingDomainRequired is returned when a campaign is started without a
	// verified sending domain.
	ErrSendingDomainRequired = apperr.NewIncorrectInput("sending-domain-required",
		"a verified sending domain is required to send")

	// ErrSendingDomainNotVerified is returned when the selected sending domain
	// is not yet verified.
	ErrSendingDomainNotVerified = apperr.NewIncorrectInput("sending-domain-not-verified",
		"the selected sending domain is not verified")

	// ErrCampaignNoRecipients is returned when a campaign is started with no
	// targeted lists or segments.
	ErrCampaignNoRecipients = apperr.NewIncorrectInput("campaign-no-recipients",
		"the campaign has no targeted lists or segments")

	// ErrRateLimited is returned when a send is denied by the rate limiter. The
	// tx handler maps it explicitly to 429 with a Retry-After header.
	ErrRateLimited = apperr.NewUnknown("rate-limited", "sending is temporarily rate-limited")

	// ErrLinkNotFound is returned when a tracking link UUID matches no row.
	ErrLinkNotFound = apperr.NewNotFound("link-not-found", "no such tracking link")

	// ErrRecipientSuppressed is returned when a transactional send targets an
	// address on the tenant's suppression list.
	ErrRecipientSuppressed = apperr.NewConflict("recipient_suppressed",
		"the recipient is suppressed and cannot be mailed")

	// ErrQuotaExceeded is returned when a send would exceed the tenant's plan
	// allowance under a block-mode plan, or the tenant has no active
	// subscription.
	ErrQuotaExceeded = apperr.NewForbidden("quota_exceeded",
		"the tenant's send allowance is exhausted")

	// ErrTenantSuspended is returned when the tenant's subscription is
	// suspended for non-payment, blocking all sends.
	ErrTenantSuspended = apperr.NewForbidden("tenant_suspended",
		"the tenant's subscription is suspended for non-payment")

	// ErrCampaignNotSent is returned when archive-visibility is enabled on a
	// campaign whose send has never begun — a draft has no audience-facing
	// content to archive.
	ErrCampaignNotSent = apperr.NewConflict("campaign_not_sent",
		"only a sent campaign can be made archive-visible")

	// ErrStaleRow is returned when a visual save is attempted with an
	// ifUnmodifiedSince timestamp that does not match the row's current
	// updated_at — the row was changed in another tab or session since the
	// editor loaded it (per FR-009). The HTTP envelope includes the row's
	// current updatedAt so the SPA can offer Reload / Force-overwrite.
	ErrStaleRow = apperr.NewConflict("stale_row",
		"the row was changed in another tab or session since it was loaded")
)

// Package domain holds the deliverability bounded context's business types:
// inbound delivery-feedback events, the suppression list, per-tenant bounce
// settings, and the campaign analytics read models. It imports nothing from
// the app, adapters, or transport layers.
package domain

import "github.com/nikolaymatrosov/nvelope/internal/platform/apperr"

// Typed deliverability-domain errors. Each carries the stable response slug and
// the transport-agnostic category; internal/api/errmap.go maps the category to
// an HTTP status in one place.
var (
	// ErrSuppressionNotFound is returned when no suppression entry matches an
	// address for the tenant.
	ErrSuppressionNotFound = apperr.NewNotFound("suppression_not_found",
		"no such suppression entry")

	// ErrRecipientSuppressed is returned when a send targets an address on the
	// tenant's suppression list.
	ErrRecipientSuppressed = apperr.NewConflict("recipient_suppressed",
		"the recipient is suppressed and cannot be mailed")

	// ErrValidationFailed is returned when an email or a bounce-setting value
	// fails validation.
	ErrValidationFailed = apperr.NewIncorrectInput("validation_failed",
		"the request failed validation")
)

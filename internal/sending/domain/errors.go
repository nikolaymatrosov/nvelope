// Package domain holds the sending bounded context's business types: the
// sending-domain aggregate and the domain-owned interfaces its use cases
// depend on. It imports nothing from the app, adapters, or transport layers.
package domain

import "github.com/nikolaymatrosov/nvelope/internal/platform/apperr"

// Typed sending-domain errors. Each carries the stable response slug and the
// transport-agnostic category; internal/api/errmap.go maps the category (and a
// few slug overrides) to an HTTP status in one place.
var (
	// ErrDomainInvalid is returned when a domain name fails validation.
	ErrDomainInvalid = apperr.NewIncorrectInput("domain-invalid",
		"that is not a valid domain name")

	// ErrDomainAlreadyExists is returned when registering a domain the tenant
	// already has.
	ErrDomainAlreadyExists = apperr.NewConflict("domain-already-exists",
		"that domain is already registered")

	// ErrDomainNotFound is returned when no sending domain matches a lookup.
	ErrDomainNotFound = apperr.NewNotFound("domain-not-found", "no such sending domain")

	// ErrDomainNotPending is returned when an action valid only for a pending
	// domain is attempted on a verified or failed one.
	ErrDomainNotPending = apperr.NewConflict("domain-not-pending",
		"this domain is no longer awaiting verification")

	// ErrProvisioningFailed is returned when the mail provider rejects a domain
	// provisioning request.
	ErrProvisioningFailed = apperr.NewUnknown("provisioning-failed",
		"the mail provider could not provision this domain")
)

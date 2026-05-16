package domain

import "github.com/nikolaymatrosov/nvelope/internal/platform/apperr"

// Typed tenant-domain errors. They carry the stable response slug and the
// transport-agnostic category; internal/api/errmap.go maps the category to an
// HTTP status in one place.
var (
	// ErrSlugTaken is returned when creating a tenant whose slug is in use.
	ErrSlugTaken = apperr.NewConflict("slug_taken", "that workspace address is already in use")

	// ErrTenantNotFound is returned when no tenant matches a lookup.
	ErrTenantNotFound = apperr.NewNotFound("tenant_not_found", "no such tenant")

	// ErrNotMember is returned when a user is not a member of a tenant. It
	// carries the same slug and message as ErrTenantNotFound so a non-member
	// and an unknown tenant produce a byte-identical, opaque 404 response.
	ErrNotMember = apperr.NewNotFound("tenant_not_found", "no such tenant")

	// ErrInvitationExists is returned when a pending invitation for the same
	// email already exists in the tenant.
	ErrInvitationExists = apperr.NewConflict("invitation_exists",
		"a pending invitation for that email already exists")

	// ErrInvitationNotFound is returned when no usable invitation matches a
	// lookup. It is deliberately opaque — unknown, expired, revoked, and
	// already-accepted invitations are indistinguishable.
	ErrInvitationNotFound = apperr.NewNotFound("invitation_not_found", "this invitation is not valid")
)

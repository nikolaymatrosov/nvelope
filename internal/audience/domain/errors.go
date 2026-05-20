// Package domain holds the audience bounded context's business types: lists,
// subscribers, the membership link between them, segments, and import/export
// jobs. It imports nothing from the app, adapters, or transport layers.
package domain

import "github.com/nikolaymatrosov/nvelope/internal/platform/apperr"

// Typed audience-domain errors. Each carries the stable response slug and the
// transport-agnostic category; internal/api/errmap.go maps the category to an
// HTTP status in one place.
var (
	// ErrListNotFound is returned when no list matches a lookup.
	ErrListNotFound = apperr.NewNotFound("list_not_found", "no such list")

	// ErrListNameTaken is returned when creating or renaming a list to a name
	// already used within the tenant.
	ErrListNameTaken = apperr.NewConflict("list_name_taken", "a list with that name already exists")

	// ErrSubscriberNotFound is returned when no subscriber matches a lookup.
	ErrSubscriberNotFound = apperr.NewNotFound("subscriber_not_found", "no such subscriber")

	// ErrSubscriberEmailTaken is returned when creating a subscriber whose
	// email is already used within the tenant.
	ErrSubscriberEmailTaken = apperr.NewConflict("subscriber_email_taken",
		"a subscriber with that email already exists")

	// ErrMembershipNotFound is returned when no membership links a subscriber
	// and a list.
	ErrMembershipNotFound = apperr.NewNotFound("membership_not_found",
		"that subscriber is not on that list")

	// ErrMembershipExists is returned when attaching a subscriber to a list it
	// is already on.
	ErrMembershipExists = apperr.NewConflict("membership_exists",
		"that subscriber is already on that list")

	// ErrJobNotFound is returned when no import/export job matches a lookup.
	ErrJobNotFound = apperr.NewNotFound("job_not_found", "no such job")

	// ErrJobNotReady is returned when an export download is requested before
	// the job has completed.
	ErrJobNotReady = apperr.NewConflict("job_not_ready", "this job has not finished yet")

	// ErrSubscriptionPageNotFound is returned when no subscription page matches
	// a lookup, or the page is inactive.
	ErrSubscriptionPageNotFound = apperr.NewNotFound("subscription_page_not_found",
		"no such subscription page")

	// ErrSubscriptionPageSlugTaken is returned when creating or renaming a
	// subscription page to a slug already used within the tenant.
	ErrSubscriptionPageSlugTaken = apperr.NewConflict("subscription_page_slug_taken",
		"a subscription page with that slug already exists")

	// ErrPendingSubscriptionNotFound is returned when no pending subscription
	// matches a lookup.
	ErrPendingSubscriptionNotFound = apperr.NewNotFound("pending_subscription_not_found",
		"no such pending subscription")

	// ErrConfirmationExpired is returned when a confirmation link is followed
	// after its expiry.
	ErrConfirmationExpired = apperr.NewIncorrectInput("confirmation_expired",
		"this confirmation link has expired")

	// ErrSubmissionThrottled is returned when a public subscription form is
	// submitted too often for the same address or source.
	ErrSubmissionThrottled = apperr.NewIncorrectInput("submission_throttled",
		"please wait a moment before trying again")

	// ErrAddressSuppressed is returned when a confirmation would re-subscribe
	// an address the tenant has suppressed.
	ErrAddressSuppressed = apperr.NewIncorrectInput("address_suppressed",
		"this address cannot be subscribed")

	// ErrSendingDomainNotFound is returned when a subscription page references
	// a sending domain that is not the tenant's.
	ErrSendingDomainNotFound = apperr.NewNotFound("sending_domain_not_found",
		"no such sending domain")

	// ErrFieldNotFound is returned when no subscriber custom-field matches a
	// lookup.
	ErrFieldNotFound = apperr.NewNotFound("subscriber_field_not_found",
		"no such subscriber field")

	// ErrFieldSlugTaken is returned when creating a custom field whose slug is
	// already in use within the tenant.
	ErrFieldSlugTaken = apperr.NewConflict("subscriber_field_slug_taken",
		"a subscriber field with that slug already exists")

	// ErrFieldBuiltinSlug is returned when a tenant tries to create a custom
	// field whose slug collides with a built-in pseudo-row (email, name,
	// first_name, last_name, state).
	ErrFieldBuiltinSlug = apperr.NewConflict("subscriber_field_builtin_slug",
		"that slug is reserved for a built-in field")

	// ErrFieldBuiltin is returned when a tenant tries to mutate or delete a
	// built-in pseudo-row.
	ErrFieldBuiltin = apperr.NewConflict("subscriber_field_builtin",
		"built-in subscriber fields cannot be edited or deleted")

	// ErrFieldInvalidSlug is returned when a slug fails the canonical regex.
	ErrFieldInvalidSlug = apperr.NewIncorrectInput("subscriber_field_invalid_slug",
		"slug must match ^[a-z][a-z0-9_]{0,62}$")

	// ErrFieldInvalidDisplayName is returned when a display name is empty or
	// exceeds 128 characters.
	ErrFieldInvalidDisplayName = apperr.NewIncorrectInput("subscriber_field_invalid_display_name",
		"display name must be between 1 and 128 characters")

	// ErrFieldInvalidType is returned when the supplied field type is not in
	// the known set.
	ErrFieldInvalidType = apperr.NewIncorrectInput("subscriber_field_invalid_type",
		"type must be one of: text, number, date, boolean, url")

	// ErrFieldReorderIncomplete is returned when a reorder request does not
	// cover every non-built-in field id exactly once.
	ErrFieldReorderIncomplete = apperr.NewIncorrectInput("subscriber_field_reorder_incomplete",
		"reorder must list every custom field exactly once")
)

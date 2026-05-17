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
)

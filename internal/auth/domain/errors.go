package domain

import "github.com/nikolaymatrosov/nvelope/internal/platform/apperr"

// Typed auth-domain errors. They carry the stable response slug and the
// transport-agnostic category; internal/api/errmap.go maps the category to an
// HTTP status in one place.
var (
	// ErrEmailTaken is returned when creating a user whose email already exists.
	ErrEmailTaken = apperr.NewConflict("email_taken", "that email is already registered")

	// ErrUserNotFound is returned when no user matches a lookup. It is an
	// internal signal — the login flow translates it to ErrInvalidCredentials so
	// account existence is not leaked.
	ErrUserNotFound = apperr.NewNotFound("user_not_found", "user not found")

	// ErrSessionInvalid is returned when a token does not resolve to a live
	// session. It is an internal signal — the session middleware treats it as an
	// unauthenticated request.
	ErrSessionInvalid = apperr.NewAuthorization("session_invalid", "session invalid or expired")

	// ErrInvalidCredentials is returned by the login flow for an unknown email
	// or a wrong password — identical for both, to resist account enumeration.
	ErrInvalidCredentials = apperr.NewAuthorization("invalid_credentials", "invalid email or password")
)

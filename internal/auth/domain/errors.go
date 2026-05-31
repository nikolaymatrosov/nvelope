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

	// ErrEmailDomainNotAllowed is returned when a registration's email domain
	// is not on the configured allowlist.
	ErrEmailDomainNotAllowed = apperr.NewIncorrectInput(
		"email_domain_not_allowed", "registration from this email domain is not allowed")

	// ErrEmailNotVerified is returned by the login flow when the password is
	// correct but the account has not yet verified its email address. It is
	// returned only after the password check, so it never leaks account
	// existence.
	ErrEmailNotVerified = apperr.NewForbidden(
		"email_not_verified", "verify your email address before signing in")

	// ErrVerificationLinkInvalid is returned when an email-verification link is
	// unknown or expired. It deliberately does not distinguish the two, so a
	// verification response cannot be used to probe for accounts.
	ErrVerificationLinkInvalid = apperr.NewIncorrectInput(
		"verification_link_invalid", "this verification link is invalid or has expired")

	// ErrVerificationResendThrottled is returned when an account has requested
	// too many verification-email resends in a short period. internal/api maps
	// its slug to HTTP 429.
	ErrVerificationResendThrottled = apperr.NewIncorrectInput(
		"verification_resend_throttled",
		"too many verification emails requested — please wait before trying again")
)

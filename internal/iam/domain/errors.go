package domain

import "github.com/nikolaymatrosov/nvelope/internal/platform/apperr"

// Typed iam-domain errors. Each carries the stable response slug and the
// transport-agnostic category; internal/api/errmap.go maps the category to an
// HTTP status in one place.
var (
	// ErrRoleNotFound is returned when no role matches a lookup.
	ErrRoleNotFound = apperr.NewNotFound("role_not_found", "no such role")

	// ErrRoleNameTaken is returned when creating or renaming a role to a name
	// already used within the tenant.
	ErrRoleNameTaken = apperr.NewConflict("role_name_taken", "a role with that name already exists")

	// ErrRoleInUse is returned when deleting a role that is still assigned.
	ErrRoleInUse = apperr.NewConflict("role_in_use",
		"that role is still assigned — reassign its holders first")

	// ErrUserNotFound is returned when no tenant-plane user matches a lookup.
	ErrUserNotFound = apperr.NewNotFound("user_not_found", "no such user")

	// ErrSessionNotFound is returned when no session matches a lookup.
	ErrSessionNotFound = apperr.NewNotFound("session_not_found", "no such session")

	// ErrUnauthenticated is returned when a credential resolves to no valid
	// principal.
	ErrUnauthenticated = apperr.NewAuthorization("unauthenticated",
		"a valid session or API key is required")

	// ErrTOTPRequired is returned when a guarded action is attempted with a
	// session still awaiting its TOTP challenge.
	ErrTOTPRequired = apperr.NewAuthorization("totp_required",
		"two-factor authentication is required to continue")

	// ErrAPIKeyNotFound is returned when no API key matches a lookup.
	ErrAPIKeyNotFound = apperr.NewNotFound("api_key_not_found", "no such API key")

	// ErrTOTPInvalidCode is returned when a TOTP enrolment or challenge code
	// does not validate against the user's secret.
	ErrTOTPInvalidCode = apperr.NewIncorrectInput("totp_invalid_code",
		"that verification code is not valid")
)

// Forbidden builds a Forbidden-category error naming the missing permission,
// e.g. permission "lists:manage" → slug "forbidden-lists-manage".
func Forbidden(p Permission) *apperr.Error {
	slug := "forbidden-" + sanitizePermission(string(p))
	return apperr.NewForbidden(slug, "you do not have permission to "+string(p))
}

// sanitizePermission turns a resource:action string into a slug-safe token.
func sanitizePermission(p string) string {
	out := make([]byte, 0, len(p))
	for i := 0; i < len(p); i++ {
		c := p[i]
		if c == ':' {
			c = '-'
		}
		out = append(out, c)
	}
	return string(out)
}

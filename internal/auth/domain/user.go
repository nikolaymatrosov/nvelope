package domain

import (
	"strings"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

// User is a platform identity. It never carries the password hash — the hash
// stays inside the persistence boundary.
type User struct {
	id              string
	email           Email
	name            string
	locale          Locale
	emailVerifiedAt *time.Time
}

// NewUser builds a new platform user. A new user has no id until it is
// persisted; the repository assigns it. NewUser validates the name; the email
// is already validated by its value object.
func NewUser(email Email, name string) (*User, error) {
	if email.IsZero() {
		return nil, apperr.NewIncorrectInput(
			"validation_failed", "a valid email address is required")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, apperr.NewIncorrectInput("validation_failed", "name is required")
	}
	return &User{email: email, name: name}, nil
}

// HydrateUser reconstructs a User from a persisted row. Persistence only — it
// is not a constructor and does not re-run validation; the email is trusted
// because it was validated on the write path. An empty or unrecognised locale
// hydrates to the unset Locale. A nil emailVerifiedAt hydrates an unverified
// account.
func HydrateUser(id, email, name, locale string, emailVerifiedAt *time.Time) *User {
	return &User{
		id:              id,
		email:           Email{value: email},
		name:            name,
		locale:          HydrateLocale(locale),
		emailVerifiedAt: emailVerifiedAt,
	}
}

// ID returns the database-assigned identifier, empty for an unpersisted user.
func (u *User) ID() string { return u.id }

// Email returns the user's email address.
func (u *User) Email() Email { return u.email }

// Name returns the user's display name.
func (u *User) Name() string { return u.name }

// Locale returns the user's chosen interface language, the unset Locale when
// the user has never explicitly chosen one.
func (u *User) Locale() Locale { return u.locale }

// SetLocale changes the user's interface-language preference. The locale is a
// value object, so it is already valid.
func (u *User) SetLocale(l Locale) { u.locale = l }

// IsEmailVerified reports whether the user has confirmed ownership of their
// email address.
func (u *User) IsEmailVerified() bool { return u.emailVerifiedAt != nil }

// EmailVerifiedAt returns the instant the user verified their email address,
// or nil when the account is still unverified.
func (u *User) EmailVerifiedAt() *time.Time { return u.emailVerifiedAt }

// MarkEmailVerified records that the user verified their email address at now.
// It is idempotent: an already-verified user keeps its original instant.
func (u *User) MarkEmailVerified(now time.Time) {
	if u.emailVerifiedAt == nil {
		u.emailVerifiedAt = &now
	}
}

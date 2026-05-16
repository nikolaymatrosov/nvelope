package domain

import (
	"strings"

	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

// User is a platform identity. It never carries the password hash — the hash
// stays inside the persistence boundary.
type User struct {
	id    string
	email Email
	name  string
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
// because it was validated on the write path.
func HydrateUser(id, email, name string) *User {
	return &User{id: id, email: Email{value: email}, name: name}
}

// ID returns the database-assigned identifier, empty for an unpersisted user.
func (u *User) ID() string { return u.id }

// Email returns the user's email address.
func (u *User) Email() Email { return u.email }

// Name returns the user's display name.
func (u *User) Name() string { return u.name }

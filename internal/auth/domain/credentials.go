package domain

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

const (
	minPasswordLen = 8
	maxPasswordLen = 72 // bcrypt's input limit
)

// emailRe matches a plausible address shape. It is intentionally permissive:
// the authoritative check is a confirmation email, not a regex.
var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// Email is a validated email address value object. Its zero value is invalid;
// build one only through NewEmail.
type Email struct {
	value string
}

// NewEmail trims surrounding whitespace and validates the address shape.
func NewEmail(raw string) (Email, error) {
	normalized := strings.TrimSpace(raw)
	if !emailRe.MatchString(normalized) {
		return Email{}, apperr.NewIncorrectInput(
			"validation_failed", "a valid email address is required")
	}
	return Email{value: normalized}, nil
}

// String returns the normalized address.
func (e Email) String() string { return e.value }

// Domain returns the address's domain — the portion after the "@" —
// lower-cased. It is the empty string for the zero-value Email.
func (e Email) Domain() string {
	at := strings.LastIndexByte(e.value, '@')
	if at < 0 {
		return ""
	}
	return strings.ToLower(e.value[at+1:])
}

// IsZero reports whether e is the unset zero value.
func (e Email) IsZero() bool { return e.value == "" }

// Password is a validated plaintext password. It is transient — it is handed
// to the password hasher and never stored on an entity.
type Password struct {
	value string
}

// NewPassword validates that plaintext is within bcrypt's usable length bounds.
func NewPassword(plaintext string) (Password, error) {
	if n := len(plaintext); n < minPasswordLen || n > maxPasswordLen {
		return Password{}, apperr.NewIncorrectInput("validation_failed", fmt.Sprintf(
			"password must be between %d and %d characters", minPasswordLen, maxPasswordLen))
	}
	return Password{value: plaintext}, nil
}

// String returns the plaintext password for hashing.
func (p Password) String() string { return p.value }

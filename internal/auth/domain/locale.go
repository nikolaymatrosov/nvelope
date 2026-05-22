package domain

import (
	"strings"

	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

// Locale is a supported interface language for the application UI. It is a
// value object: the zero value is the "unset" state — a user who has never
// explicitly chosen a language — and a non-zero value is always one of the
// supported codes, so an unsupported locale is unrepresentable.
type Locale struct {
	code string
}

// Supported locale codes.
const (
	localeEN = "en"
	localeRU = "ru"
)

var supportedLocales = map[string]struct{}{
	localeEN: {},
	localeRU: {},
}

// DefaultLocale is the locale used whenever no other locale can be determined.
var DefaultLocale = Locale{code: localeEN}

// NewLocale builds a Locale from a code, rejecting anything outside the
// supported set. The input is trimmed and lower-cased.
func NewLocale(code string) (Locale, error) {
	normalized := strings.ToLower(strings.TrimSpace(code))
	if _, ok := supportedLocales[normalized]; !ok {
		return Locale{}, apperr.NewIncorrectInput(
			"unsupported_locale", "unsupported interface language")
	}
	return Locale{code: normalized}, nil
}

// HydrateLocale reconstructs a Locale from a persisted value. Persistence only
// — it is not a constructor and does not error. An unrecognised or empty
// stored value hydrates to the zero (unset) Locale, so a locale later removed
// from the supported set degrades to the default rather than failing a load.
func HydrateLocale(code string) Locale {
	normalized := strings.ToLower(strings.TrimSpace(code))
	if _, ok := supportedLocales[normalized]; !ok {
		return Locale{}
	}
	return Locale{code: normalized}
}

// String returns the locale code, empty for the unset Locale.
func (l Locale) String() string { return l.code }

// IsZero reports whether the locale is unset.
func (l Locale) IsZero() bool { return l.code == "" }

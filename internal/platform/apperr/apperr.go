package apperr

import (
	"errors"
	"fmt"
)

// Category is the transport-agnostic classification of an application error.
// internal/api/errmap.go is the single place that maps a Category to an HTTP
// status code; domain and application code never know about HTTP.
type Category int

const (
	// Unknown is an unexpected, uncategorized failure — mapped to a 500.
	Unknown Category = iota
	// IncorrectInput is input that failed a validation rule — mapped to a 422.
	IncorrectInput
	// Conflict is a request that clashes with existing state — mapped to a 409.
	Conflict
	// NotFound is a missing (or opaquely denied) resource — mapped to a 404.
	NotFound
	// Authorization is a missing or invalid credential — mapped to a 401.
	Authorization
)

func (c Category) String() string {
	switch c {
	case IncorrectInput:
		return "incorrect-input"
	case Conflict:
		return "conflict"
	case NotFound:
		return "not-found"
	case Authorization:
		return "authorization"
	default:
		return "unknown"
	}
}

// Error is an application error that crosses a domain boundary. It carries a
// stable machine-readable slug (the token surfaced in API responses) and a
// Category, plus a human-readable message and an optional wrapped cause.
type Error struct {
	slug     string
	category Category
	message  string
	cause    error
}

// New builds an Error in the given category.
func New(category Category, slug, message string) *Error {
	return &Error{slug: slug, category: category, message: message}
}

// NewIncorrectInput builds an IncorrectInput-category Error.
func NewIncorrectInput(slug, message string) *Error { return New(IncorrectInput, slug, message) }

// NewConflict builds a Conflict-category Error.
func NewConflict(slug, message string) *Error { return New(Conflict, slug, message) }

// NewNotFound builds a NotFound-category Error.
func NewNotFound(slug, message string) *Error { return New(NotFound, slug, message) }

// NewAuthorization builds an Authorization-category Error.
func NewAuthorization(slug, message string) *Error { return New(Authorization, slug, message) }

// NewUnknown builds an Unknown-category Error.
func NewUnknown(slug, message string) *Error { return New(Unknown, slug, message) }

// Wrap returns an Error in the given category carrying cause, so the original
// failure is available to errors.Unwrap and logging without its text reaching
// the API response.
func Wrap(cause error, category Category, slug, message string) *Error {
	return &Error{slug: slug, category: category, message: message, cause: cause}
}

func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %v", e.slug, e.cause)
	}
	if e.message != "" {
		return e.message
	}
	return e.slug
}

// Slug returns the stable machine-readable token for this error.
func (e *Error) Slug() string { return e.slug }

// Category returns the transport-agnostic classification of this error.
func (e *Error) Category() Category { return e.category }

// Message returns the human-readable message safe to show the caller.
func (e *Error) Message() string { return e.message }

// Unwrap returns the wrapped cause, if any.
func (e *Error) Unwrap() error { return e.cause }

// Is reports whether target is an Error with the same slug, so sentinel
// comparison with errors.Is survives wrapping and message overrides.
func (e *Error) Is(target error) bool {
	var t *Error
	if !errors.As(target, &t) {
		return false
	}
	return e.slug == t.slug
}

// WithMessage returns a copy of e with a different human-readable message,
// preserving the slug, category, and cause. It is used where one slug needs
// context-specific wording.
func (e *Error) WithMessage(message string) *Error {
	c := *e
	c.message = message
	return &c
}

// As extracts the *Error from err's chain, reporting whether one was found.
func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}

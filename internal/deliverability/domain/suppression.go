package domain

import (
	"regexp"
	"strings"
	"time"
)

// emailPattern is a deliberately permissive syntactic check for an email
// address — enough to reject obvious garbage before suppression.
var emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// normalizeEmail lower-cases and trims an address, returning whether it is
// syntactically plausible.
func normalizeEmail(email string) (string, bool) {
	email = strings.ToLower(strings.TrimSpace(email))
	return email, emailPattern.MatchString(email)
}

// SuppressionEntry is an address suppressed for a tenant — it must not be
// mailed. It is a tenant-plane aggregate reached only through its repository's
// RLS-bound transaction.
type SuppressionEntry struct {
	id            string
	tenantID      string
	email         string
	reason        SuppressionReason
	sourceEventID string
	suppressedAt  time.Time
	note          string
}

// NewSuppressionEntry builds an automatic suppression entry from a delivery
// event, rejecting an invalid email or an unknown reason. sourceEventID may be
// empty.
func NewSuppressionEntry(tenantID, email string, reason SuppressionReason,
	sourceEventID string) (*SuppressionEntry, error) {

	if tenantID == "" {
		return nil, ErrValidationFailed.WithMessage("a tenant is required")
	}
	normalized, ok := normalizeEmail(email)
	if !ok {
		return nil, ErrValidationFailed.WithMessage("a valid email address is required")
	}
	if !validReason(reason) {
		return nil, ErrValidationFailed.WithMessage("unknown suppression reason")
	}
	return &SuppressionEntry{
		tenantID:      tenantID,
		email:         normalized,
		reason:        reason,
		sourceEventID: sourceEventID,
	}, nil
}

// NewManualSuppression builds an operator-added suppression entry, rejecting an
// invalid email.
func NewManualSuppression(tenantID, email, note string) (*SuppressionEntry, error) {
	e, err := NewSuppressionEntry(tenantID, email, ReasonManual, "")
	if err != nil {
		return nil, err
	}
	e.note = strings.TrimSpace(note)
	return e, nil
}

// HydrateSuppressionEntry reconstructs a suppression entry from a persisted
// row. Persistence only — it performs no validation and is not a constructor.
func HydrateSuppressionEntry(id, tenantID, email string, reason SuppressionReason,
	sourceEventID string, suppressedAt time.Time, note string) *SuppressionEntry {

	return &SuppressionEntry{
		id: id, tenantID: tenantID, email: email, reason: reason,
		sourceEventID: sourceEventID, suppressedAt: suppressedAt, note: note,
	}
}

// ID returns the database-assigned id.
func (e *SuppressionEntry) ID() string { return e.id }

// TenantID returns the owning tenant's id.
func (e *SuppressionEntry) TenantID() string { return e.tenantID }

// Email returns the lower-cased suppressed address.
func (e *SuppressionEntry) Email() string { return e.email }

// Reason returns why the address was suppressed.
func (e *SuppressionEntry) Reason() SuppressionReason { return e.reason }

// SourceEventID returns the delivery event that triggered an automatic
// suppression, or "" for a manual entry.
func (e *SuppressionEntry) SourceEventID() string { return e.sourceEventID }

// SuppressedAt returns when the address was suppressed.
func (e *SuppressionEntry) SuppressedAt() time.Time { return e.suppressedAt }

// Note returns the optional operator note.
func (e *SuppressionEntry) Note() string { return e.note }

// SuppressionFilter narrows a suppression-list query.
type SuppressionFilter struct {
	// Reason, when set, restricts the page to entries with this reason.
	Reason SuppressionReason
	// EmailLike, when set, restricts the page to addresses containing this
	// substring.
	EmailLike string
	// Cursor is the opaque pagination cursor; empty starts at the first page.
	Cursor string
	// Limit bounds the page size.
	Limit int
}

// validReason reports whether r is a known suppression reason.
func validReason(r SuppressionReason) bool {
	switch r {
	case ReasonHardBounce, ReasonComplaint, ReasonManual:
		return true
	default:
		return false
	}
}

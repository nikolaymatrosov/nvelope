package domain

import (
	"regexp"
	"strings"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

// FieldType is the data shape a subscriber custom field holds.
type FieldType string

const (
	// FieldTypeText is a free-form text value.
	FieldTypeText FieldType = "text"
	// FieldTypeNumber is a numeric value (no fixed precision; storage is text).
	FieldTypeNumber FieldType = "number"
	// FieldTypeDate is a date (no time) value.
	FieldTypeDate FieldType = "date"
	// FieldTypeBoolean is a true/false value.
	FieldTypeBoolean FieldType = "boolean"
	// FieldTypeURL is an HTTP(S) URL.
	FieldTypeURL FieldType = "url"
)

// validFieldType reports whether t is a known field type.
func validFieldType(t FieldType) bool {
	switch t {
	case FieldTypeText, FieldTypeNumber, FieldTypeDate, FieldTypeBoolean, FieldTypeURL:
		return true
	}
	return false
}

// slugRE is the canonical regex for a subscriber-field slug. Mirrors the
// CHECK constraint in migration 000020. Starts with a lowercase letter; up
// to 62 more lowercase letters, digits, or underscores.
var slugRE = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// Field is a tenant-scoped subscriber custom-field definition. It feeds the
// visual editor's merge-tag picker, the Phase 6 subscription-page "visible
// profile fields" picker, and the send-time placeholder substitutor.
//
// Built-in fields (email, name, first_name, last_name, state) are surfaced
// as pseudo-rows by the query layer with builtIn=true; they cannot be
// constructed by NewField and cannot be persisted.
type Field struct {
	id           string
	tenantID     string
	slug         string
	displayName  string
	fieldType    FieldType
	defaultValue string
	position     int
	builtIn      bool
	createdAt    time.Time
	updatedAt    time.Time
}

// NewField builds a tenant-owned subscriber custom-field definition,
// rejecting any invariant violation. builtIn is always false on construction
// — only HydrateField may set it for the package-level pseudo-rows.
func NewField(tenantID, slug, displayName string, t FieldType, defaultValue string, position int) (*Field, error) {
	if tenantID == "" {
		return nil, apperr.NewIncorrectInput("validation_failed", "a tenant is required")
	}
	slug = strings.TrimSpace(slug)
	displayName = strings.TrimSpace(displayName)
	if !slugRE.MatchString(slug) {
		return nil, ErrFieldInvalidSlug
	}
	if !validFieldType(t) {
		return nil, ErrFieldInvalidType
	}
	if displayName == "" || len(displayName) > 128 {
		return nil, ErrFieldInvalidDisplayName
	}
	return &Field{
		tenantID:     tenantID,
		slug:         slug,
		displayName:  displayName,
		fieldType:    t,
		defaultValue: defaultValue,
		position:     position,
	}, nil
}

// HydrateField reconstructs a field from a persisted row OR builds one of the
// package-level built-in pseudo-rows. Persistence/composition only — no
// validation. Callers outside the package MUST go through NewField for
// operator-supplied input.
func HydrateField(id, tenantID, slug, displayName string, t FieldType, defaultValue string, position int, builtIn bool, createdAt, updatedAt time.Time) *Field {
	return &Field{
		id:           id,
		tenantID:     tenantID,
		slug:         slug,
		displayName:  displayName,
		fieldType:    t,
		defaultValue: defaultValue,
		position:     position,
		builtIn:      builtIn,
		createdAt:    createdAt,
		updatedAt:    updatedAt,
	}
}

// ID returns the database-assigned id. Empty for built-in pseudo-rows that
// have not been persisted (they use a synthetic id assigned by the query
// layer instead).
func (f *Field) ID() string { return f.id }

// TenantID returns the owning tenant's id. Empty for built-in pseudo-rows.
func (f *Field) TenantID() string { return f.tenantID }

// Slug returns the placeholder key — operators write
// `{{ subscriber.<slug> }}` in campaign and template content.
func (f *Field) Slug() string { return f.slug }

// DisplayName returns the human-readable label.
func (f *Field) DisplayName() string { return f.displayName }

// Type returns the field's data type.
func (f *Field) Type() FieldType { return f.fieldType }

// DefaultValue returns the optional default value (interpreted per Type by
// the substitutor).
func (f *Field) DefaultValue() string { return f.defaultValue }

// Position returns the operator-managed display ordering.
func (f *Field) Position() int { return f.position }

// BuiltIn reports whether this is a built-in pseudo-row.
func (f *Field) BuiltIn() bool { return f.builtIn }

// CreatedAt returns when the field was created. Zero for built-ins.
func (f *Field) CreatedAt() time.Time { return f.createdAt }

// UpdatedAt returns when the field was last changed. Zero for built-ins.
func (f *Field) UpdatedAt() time.Time { return f.updatedAt }

// Rename changes the display name, rejecting invariant violations. The slug
// is immutable post-creation.
func (f *Field) Rename(displayName string) error {
	displayName = strings.TrimSpace(displayName)
	if displayName == "" || len(displayName) > 128 {
		return ErrFieldInvalidDisplayName
	}
	f.displayName = displayName
	return nil
}

// Retype changes the field's data type, rejecting an unknown type.
func (f *Field) Retype(t FieldType) error {
	if !validFieldType(t) {
		return ErrFieldInvalidType
	}
	f.fieldType = t
	return nil
}

// SetDefaultValue updates the optional default.
func (f *Field) SetDefaultValue(v string) {
	f.defaultValue = v
}

// Reposition updates the ordering.
func (f *Field) Reposition(p int) {
	f.position = p
}

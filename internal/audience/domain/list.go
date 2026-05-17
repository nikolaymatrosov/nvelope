package domain

import (
	"strings"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

// Visibility is whether a list is publicly discoverable or private.
type Visibility string

// OptIn is the subscription confirmation mode for a list.
type OptIn string

const (
	// VisibilityPublic marks a list as publicly discoverable.
	VisibilityPublic Visibility = "public"
	// VisibilityPrivate marks a list as private.
	VisibilityPrivate Visibility = "private"

	// OptInSingle is single opt-in: a membership is effective immediately.
	OptInSingle OptIn = "single"
	// OptInDouble is double opt-in: a membership awaits explicit confirmation.
	OptInDouble OptIn = "double"
)

// List is a named collection a subscriber can belong to. It is a tenant-plane
// aggregate reached only through the RLS-bound transaction owned by its
// repository adapter.
type List struct {
	id          string
	tenantID    string
	name        string
	description string
	visibility  Visibility
	optIn       OptIn
	tags        []string
	createdAt   time.Time
	updatedAt   time.Time
}

func validVisibility(v Visibility) bool {
	return v == VisibilityPublic || v == VisibilityPrivate
}

func validOptIn(o OptIn) bool {
	return o == OptInSingle || o == OptInDouble
}

// NewList builds a list, rejecting any invariant violation. The database
// assigns the id and timestamps, so a freshly built list carries none.
func NewList(tenantID, name, description string, visibility Visibility, optIn OptIn, tags []string) (*List, error) {
	if tenantID == "" {
		return nil, apperr.NewIncorrectInput("validation_failed", "a tenant is required")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, apperr.NewIncorrectInput("validation_failed", "list name is required")
	}
	if !validVisibility(visibility) {
		return nil, apperr.NewIncorrectInput("validation_failed", "visibility must be public or private")
	}
	if !validOptIn(optIn) {
		return nil, apperr.NewIncorrectInput("validation_failed", "opt-in must be single or double")
	}
	return &List{
		tenantID:    tenantID,
		name:        name,
		description: strings.TrimSpace(description),
		visibility:  visibility,
		optIn:       optIn,
		tags:        normalizeTags(tags),
	}, nil
}

// HydrateList reconstructs a list from a persisted row. Persistence only — it
// is not a constructor and performs no validation.
func HydrateList(id, tenantID, name, description string, visibility Visibility, optIn OptIn,
	tags []string, createdAt, updatedAt time.Time) *List {
	return &List{
		id:          id,
		tenantID:    tenantID,
		name:        name,
		description: description,
		visibility:  visibility,
		optIn:       optIn,
		tags:        tags,
		createdAt:   createdAt,
		updatedAt:   updatedAt,
	}
}

// ID returns the list's database-assigned id.
func (l *List) ID() string { return l.id }

// TenantID returns the owning tenant's id.
func (l *List) TenantID() string { return l.tenantID }

// Name returns the list name.
func (l *List) Name() string { return l.name }

// Description returns the list description.
func (l *List) Description() string { return l.description }

// Visibility returns the list visibility.
func (l *List) Visibility() Visibility { return l.visibility }

// OptIn returns the list opt-in mode.
func (l *List) OptIn() OptIn { return l.optIn }

// Tags returns the list tags.
func (l *List) Tags() []string { return l.tags }

// CreatedAt returns when the list was created.
func (l *List) CreatedAt() time.Time { return l.createdAt }

// UpdatedAt returns when the list was last changed.
func (l *List) UpdatedAt() time.Time { return l.updatedAt }

// Rename changes the list name, rejecting an empty value.
func (l *List) Rename(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return apperr.NewIncorrectInput("validation_failed", "list name is required")
	}
	l.name = name
	return nil
}

// Describe changes the list description.
func (l *List) Describe(description string) {
	l.description = strings.TrimSpace(description)
}

// Retag replaces the list tags.
func (l *List) Retag(tags []string) {
	l.tags = normalizeTags(tags)
}

// SetVisibility changes the list visibility, rejecting an unknown value.
func (l *List) SetVisibility(v Visibility) error {
	if !validVisibility(v) {
		return apperr.NewIncorrectInput("validation_failed", "visibility must be public or private")
	}
	l.visibility = v
	return nil
}

// SetOptIn changes the list opt-in mode, rejecting an unknown value.
func (l *List) SetOptIn(o OptIn) error {
	if !validOptIn(o) {
		return apperr.NewIncorrectInput("validation_failed", "opt-in must be single or double")
	}
	l.optIn = o
	return nil
}

// normalizeTags trims each tag and drops the empties, returning a non-nil
// slice so a list always has a concrete (possibly empty) tag set.
func normalizeTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

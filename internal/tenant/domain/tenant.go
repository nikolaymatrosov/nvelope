package domain

import (
	"regexp"
	"strings"

	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

// slugRe allows 3-63 characters: lowercase letters, digits, and interior
// hyphens; the slug must start and end alphanumeric.
var slugRe = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{1,61}[a-z0-9])$`)

// reservedSlugs would collide with fixed top-level path segments.
var reservedSlugs = map[string]bool{
	"api": true, "t": true, "admin": true, "login": true, "signup": true,
	"invite": true, "static": true, "assets": true, "healthz": true,
}

// Slug is a validated, non-reserved tenant slug — a workspace's URL path
// segment. Its zero value is invalid; build one only through NewSlug or
// DeriveSlug.
type Slug struct {
	value string
}

// NewSlug lowercases, trims, and validates raw as a tenant slug.
func NewSlug(raw string) (Slug, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if !slugRe.MatchString(s) {
		return Slug{}, apperr.NewIncorrectInput("validation_failed",
			"slug must be 3-63 characters: lowercase letters, digits, and hyphens")
	}
	if reservedSlugs[s] {
		return Slug{}, apperr.NewIncorrectInput("validation_failed",
			"that slug is reserved; please choose another")
	}
	return Slug{value: s}, nil
}

// DeriveSlug builds a slug from a workspace name, then validates it.
func DeriveSlug(name string) (Slug, error) {
	var b strings.Builder
	prevHyphen := false
	for _, r := range strings.ToLower(name) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevHyphen = false
		default:
			if !prevHyphen && b.Len() > 0 {
				b.WriteByte('-')
				prevHyphen = true
			}
		}
	}
	return NewSlug(strings.Trim(b.String(), "-"))
}

// String returns the slug text.
func (s Slug) String() string { return s.value }

// IsZero reports whether s is the unset zero value.
func (s Slug) IsZero() bool { return s.value == "" }

// TenantStatus is a tenant's lifecycle status. The only value this phase is
// StatusActive.
type TenantStatus string

// StatusActive is the status of a usable workspace.
const StatusActive TenantStatus = "active"

// Tenant is an isolated workspace — the root of the tenant plane.
type Tenant struct {
	id     string
	slug   Slug
	name   string
	status TenantStatus
}

// NewTenant builds a new workspace. When slug is empty it is derived from the
// name; otherwise the supplied slug is validated.
func NewTenant(name, slug string) (*Tenant, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, apperr.NewIncorrectInput("validation_failed", "tenant name is required")
	}
	var (
		s   Slug
		err error
	)
	if strings.TrimSpace(slug) == "" {
		s, err = DeriveSlug(name)
	} else {
		s, err = NewSlug(slug)
	}
	if err != nil {
		return nil, err
	}
	return &Tenant{name: name, slug: s, status: StatusActive}, nil
}

// HydrateTenant reconstructs a Tenant from a persisted row. Persistence only —
// it is not a constructor.
func HydrateTenant(id, slug, name, status string) *Tenant {
	return &Tenant{id: id, slug: Slug{value: slug}, name: name, status: TenantStatus(status)}
}

// ID returns the database-assigned identifier.
func (t *Tenant) ID() string { return t.id }

// Slug returns the workspace slug.
func (t *Tenant) Slug() Slug { return t.slug }

// Name returns the workspace name.
func (t *Tenant) Name() string { return t.name }

// Status returns the workspace lifecycle status.
func (t *Tenant) Status() TenantStatus { return t.status }

package tenant

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nvelope/nvelope/internal/db"
)

// Tenant is an isolated workspace.
type Tenant struct {
	ID     string `json:"id"`
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// Membership pairs a tenant with the caller's role in it.
type Membership struct {
	Tenant
	Role string `json:"role"`
}

// Member is one member of a tenant.
type Member struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Name   string `json:"name"`
	Role   string `json:"role"`
}

// ErrSlugTaken is returned when creating a tenant whose slug is in use.
var ErrSlugTaken = errors.New("tenant slug already in use")

// ErrTenantNotFound is returned when no tenant matches a lookup.
var ErrTenantNotFound = errors.New("tenant not found")

// ErrNotMember is returned when a user is not a member of a tenant.
var ErrNotMember = errors.New("user is not a member of the tenant")

// ValidationError describes a request that failed input validation. Its
// message is safe to show the user.
type ValidationError struct{ Message string }

func (e ValidationError) Error() string { return e.Message }

// slugRe allows 3-63 characters: lowercase letters, digits, and interior
// hyphens; it must start and end alphanumeric.
var slugRe = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{1,61}[a-z0-9])$`)

// reservedSlugs would collide with fixed top-level path segments.
var reservedSlugs = map[string]bool{
	"api": true, "t": true, "admin": true, "login": true, "signup": true,
	"invite": true, "static": true, "assets": true, "healthz": true,
}

// ValidateSlug reports whether slug is a usable, non-reserved tenant slug.
func ValidateSlug(slug string) error {
	if !slugRe.MatchString(slug) {
		return ValidationError{
			"slug must be 3-63 characters: lowercase letters, digits, and hyphens"}
	}
	if reservedSlugs[slug] {
		return ValidationError{"that slug is reserved; please choose another"}
	}
	return nil
}

// deriveSlug builds a candidate slug from a tenant name.
func deriveSlug(name string) string {
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
	return strings.Trim(b.String(), "-")
}

// CreateTenant creates a tenant owned by ownerID. In one transaction it
// inserts the tenant, the owner membership, and the tenant's initial
// tenant_settings row (the last under an RLS binding). slug may be empty, in
// which case it is derived from name.
func CreateTenant(ctx context.Context, pool *pgxpool.Pool, ownerID, name, slug string) (Tenant, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Tenant{}, ValidationError{"tenant name is required"}
	}
	if strings.TrimSpace(slug) == "" {
		slug = deriveSlug(name)
	}
	slug = strings.ToLower(strings.TrimSpace(slug))
	if err := ValidateSlug(slug); err != nil {
		return Tenant{}, err
	}

	var t Tenant
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`INSERT INTO tenants (slug, name) VALUES ($1, $2)
			 RETURNING id, slug, name, status`,
			slug, name).Scan(&t.ID, &t.Slug, &t.Name, &t.Status)
		if err != nil {
			if db.IsUniqueViolation(err) {
				return ErrSlugTaken
			}
			return fmt.Errorf("inserting tenant: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO platform_user_tenants (platform_user_id, tenant_id, role)
			 VALUES ($1, $2, 'owner')`,
			ownerID, t.ID); err != nil {
			return fmt.Errorf("inserting owner membership: %w", err)
		}
		// Bind the new tenant so the tenant_settings RLS WITH CHECK accepts
		// the row inserted next.
		if _, err := tx.Exec(ctx,
			"SELECT set_config('app.tenant_id', $1, true)", t.ID); err != nil {
			return fmt.Errorf("binding tenant: %w", err)
		}
		return createSettings(ctx, tx, t.ID, t.Name)
	})
	if err != nil {
		return Tenant{}, err
	}
	return t, nil
}

// GetTenantBySlug returns the tenant with the given slug, or ErrTenantNotFound.
func GetTenantBySlug(ctx context.Context, q db.Querier, slug string) (Tenant, error) {
	var t Tenant
	err := q.QueryRow(ctx,
		"SELECT id, slug, name, status FROM tenants WHERE slug = $1", slug).
		Scan(&t.ID, &t.Slug, &t.Name, &t.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Tenant{}, ErrTenantNotFound
	}
	if err != nil {
		return Tenant{}, fmt.Errorf("loading tenant: %w", err)
	}
	return t, nil
}

// GetTenantByID returns the tenant with the given id, or ErrTenantNotFound.
func GetTenantByID(ctx context.Context, q db.Querier, id string) (Tenant, error) {
	var t Tenant
	err := q.QueryRow(ctx,
		"SELECT id, slug, name, status FROM tenants WHERE id = $1", id).
		Scan(&t.ID, &t.Slug, &t.Name, &t.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Tenant{}, ErrTenantNotFound
	}
	if err != nil {
		return Tenant{}, fmt.Errorf("loading tenant: %w", err)
	}
	return t, nil
}

// ListMembershipsForUser returns every tenant the user belongs to, with the
// user's role in each.
func ListMembershipsForUser(ctx context.Context, q db.Querier, userID string) ([]Membership, error) {
	rows, err := q.Query(ctx,
		`SELECT t.id, t.slug, t.name, t.status, m.role
		 FROM platform_user_tenants m
		 JOIN tenants t ON t.id = m.tenant_id
		 WHERE m.platform_user_id = $1
		 ORDER BY m.created_at`, userID)
	if err != nil {
		return nil, fmt.Errorf("listing memberships: %w", err)
	}
	defer rows.Close()

	memberships := []Membership{}
	for rows.Next() {
		var m Membership
		if err := rows.Scan(&m.ID, &m.Slug, &m.Name, &m.Status, &m.Role); err != nil {
			return nil, fmt.Errorf("scanning membership: %w", err)
		}
		memberships = append(memberships, m)
	}
	return memberships, rows.Err()
}

// GetMembershipRole returns the user's role in the tenant, or ErrNotMember.
func GetMembershipRole(ctx context.Context, q db.Querier, userID, tenantID string) (string, error) {
	var role string
	err := q.QueryRow(ctx,
		`SELECT role FROM platform_user_tenants
		 WHERE platform_user_id = $1 AND tenant_id = $2`,
		userID, tenantID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotMember
	}
	if err != nil {
		return "", fmt.Errorf("loading membership: %w", err)
	}
	return role, nil
}

// AddMembership records userID as a member of tenantID with the given role.
// Re-adding an existing member is a no-op (ON CONFLICT DO NOTHING).
func AddMembership(ctx context.Context, q db.Querier, userID, tenantID, role string) error {
	if _, err := q.Exec(ctx,
		`INSERT INTO platform_user_tenants (platform_user_id, tenant_id, role)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (platform_user_id, tenant_id) DO NOTHING`,
		userID, tenantID, role); err != nil {
		return fmt.Errorf("inserting membership: %w", err)
	}
	return nil
}

// ListMembers returns every member of the tenant.
func ListMembers(ctx context.Context, q db.Querier, tenantID string) ([]Member, error) {
	rows, err := q.Query(ctx,
		`SELECT u.id, u.email, u.name, m.role
		 FROM platform_user_tenants m
		 JOIN platform_users u ON u.id = m.platform_user_id
		 WHERE m.tenant_id = $1
		 ORDER BY m.created_at`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing members: %w", err)
	}
	defer rows.Close()

	members := []Member{}
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.UserID, &m.Email, &m.Name, &m.Role); err != nil {
			return nil, fmt.Errorf("scanning member: %w", err)
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

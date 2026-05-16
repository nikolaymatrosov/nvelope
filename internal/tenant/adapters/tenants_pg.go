package adapters

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/db"
	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

// Tenants is the pgx-backed implementation of domain.TenantRepository.
type Tenants struct {
	pool *pgxpool.Pool
}

var _ domain.TenantRepository = (*Tenants)(nil)

// NewTenants builds a Tenants repository over the given pool.
func NewTenants(pool *pgxpool.Pool) *Tenants {
	return &Tenants{pool: pool}
}

// CreateWorkspace inserts the tenant, its owner membership, and the initial
// tenant_settings row in one transaction, returning the persisted tenant. The
// settings insert runs after the new tenant id is bound to app.tenant_id so
// the RLS WITH CHECK clause accepts it.
func (r *Tenants) CreateWorkspace(ctx context.Context, t *domain.Tenant, ownerID string) (*domain.Tenant, error) {

	var created *domain.Tenant
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		var id, slug, name, status string
		err := tx.QueryRow(ctx,
			`INSERT INTO tenants (slug, name) VALUES ($1, $2)
			 RETURNING id, slug, name, status`,
			t.Slug().String(), t.Name()).Scan(&id, &slug, &name, &status)
		if err != nil {
			if db.IsUniqueViolation(err) {
				return domain.ErrSlugTaken
			}
			return fmt.Errorf("inserting tenant: %w", err)
		}

		if _, err := tx.Exec(ctx,
			`INSERT INTO platform_user_tenants (platform_user_id, tenant_id, role)
			 VALUES ($1, $2, 'owner')`,
			ownerID, id); err != nil {
			return fmt.Errorf("inserting owner membership: %w", err)
		}

		// Bind the new tenant so the tenant_settings RLS WITH CHECK accepts the
		// row inserted next.
		if _, err := tx.Exec(ctx,
			"SELECT set_config('app.tenant_id', $1, true)", id); err != nil {
			return fmt.Errorf("binding tenant: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO tenant_settings (tenant_id, display_name) VALUES ($1, $2)`,
			id, name); err != nil {
			return fmt.Errorf("inserting tenant settings: %w", err)
		}

		created = domain.HydrateTenant(id, slug, name, status)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// GetBySlug returns the tenant with the given slug, or
// domain.ErrTenantNotFound.
func (r *Tenants) GetBySlug(ctx context.Context, slug string) (*domain.Tenant, error) {
	return r.getOne(ctx, "SELECT id, slug, name, status FROM tenants WHERE slug = $1", slug)
}

// GetByID returns the tenant with the given id, or domain.ErrTenantNotFound.
func (r *Tenants) GetByID(ctx context.Context, id string) (*domain.Tenant, error) {
	return r.getOne(ctx, "SELECT id, slug, name, status FROM tenants WHERE id = $1", id)
}

func (r *Tenants) getOne(ctx context.Context, query, arg string) (*domain.Tenant, error) {
	var id, slug, name, status string
	err := r.pool.QueryRow(ctx, query, arg).Scan(&id, &slug, &name, &status)
	if errors.Is(err, pgx.ErrNoRows) || db.IsInvalidInput(err) {
		return nil, domain.ErrTenantNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading tenant: %w", err)
	}
	return domain.HydrateTenant(id, slug, name, status), nil
}

// GetMembershipRole returns the user's role in the tenant, or
// domain.ErrNotMember.
func (r *Tenants) GetMembershipRole(ctx context.Context, userID, tenantID string) (domain.Role, error) {
	var role string
	err := r.pool.QueryRow(ctx,
		`SELECT role FROM platform_user_tenants
		 WHERE platform_user_id = $1 AND tenant_id = $2`,
		userID, tenantID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Role{}, domain.ErrNotMember
	}
	if err != nil {
		return domain.Role{}, fmt.Errorf("loading membership: %w", err)
	}
	parsed, err := domain.NewRole(role)
	if err != nil {
		return domain.Role{}, fmt.Errorf("loading membership role: %w", err)
	}
	return parsed, nil
}

// AddMembership records a membership. Re-adding an existing member is a no-op.
func (r *Tenants) AddMembership(ctx context.Context, m *domain.Membership) error {
	if _, err := r.pool.Exec(ctx,
		`INSERT INTO platform_user_tenants (platform_user_id, tenant_id, role)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (platform_user_id, tenant_id) DO NOTHING`,
		m.UserID(), m.TenantID(), m.Role().String()); err != nil {
		return fmt.Errorf("inserting membership: %w", err)
	}
	return nil
}

// ListMembershipsForUser returns every tenant the user belongs to, with the
// user's role in each, oldest membership first.
func (r *Tenants) ListMembershipsForUser(ctx context.Context, userID string) ([]domain.MembershipDetail, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT t.id, t.slug, t.name, t.status, m.role
		 FROM platform_user_tenants m
		 JOIN tenants t ON t.id = m.tenant_id
		 WHERE m.platform_user_id = $1
		 ORDER BY m.created_at`, userID)
	if err != nil {
		return nil, fmt.Errorf("listing memberships: %w", err)
	}
	defer rows.Close()

	memberships := []domain.MembershipDetail{}
	for rows.Next() {
		var id, slug, name, status, role string
		if err := rows.Scan(&id, &slug, &name, &status, &role); err != nil {
			return nil, fmt.Errorf("scanning membership: %w", err)
		}
		parsedRole, err := domain.NewRole(role)
		if err != nil {
			return nil, fmt.Errorf("scanning membership role: %w", err)
		}
		memberships = append(memberships, domain.MembershipDetail{
			Tenant: domain.HydrateTenant(id, slug, name, status),
			Role:   parsedRole,
		})
	}
	return memberships, rows.Err()
}

// ListMembers returns every member of the tenant, oldest membership first.
func (r *Tenants) ListMembers(ctx context.Context, tenantID string) ([]domain.Member, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT u.id, u.email, u.name, m.role
		 FROM platform_user_tenants m
		 JOIN platform_users u ON u.id = m.platform_user_id
		 WHERE m.tenant_id = $1
		 ORDER BY m.created_at`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing members: %w", err)
	}
	defer rows.Close()

	members := []domain.Member{}
	for rows.Next() {
		var userID, email, name, role string
		if err := rows.Scan(&userID, &email, &name, &role); err != nil {
			return nil, fmt.Errorf("scanning member: %w", err)
		}
		parsedRole, err := domain.NewRole(role)
		if err != nil {
			return nil, fmt.Errorf("scanning member role: %w", err)
		}
		members = append(members, domain.Member{
			UserID: userID, Email: email, Name: name, Role: parsedRole,
		})
	}
	return members, rows.Err()
}

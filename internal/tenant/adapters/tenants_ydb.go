package adapters

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/db"
	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

// YDBTenants is the YDB-backed implementation of domain.TenantRepository, for
// the serverless runtime. tenants is a control-plane table keyed by id. A
// membership row is keyed (tenant_id, platform_user_id), so the tenant-scoped
// reads are key-prefix lookups. ListMembershipsForUser is the one membership
// read with no tenant predicate: it lists a user's tenants through the user
// index.
type YDBTenants struct {
	driver *ydb.Driver
}

var _ domain.TenantRepository = (*YDBTenants)(nil)

// NewYDBTenants builds a YDBTenants repository over the driver.
func NewYDBTenants(d *ydb.Driver) *YDBTenants {
	return &YDBTenants{driver: d}
}

// CreateWorkspace inserts the tenant, its owner membership and the initial
// tenant_settings row in one transaction, and returns the persisted tenant.
// The id is generated before the closure, so a retried transaction writes the
// same rows. Postgres takes the status and the settings timezone from column
// defaults. YDB has none, so the statements write "active" and "UTC".
func (r *YDBTenants) CreateWorkspace(ctx context.Context, t *domain.Tenant, ownerID string) (*domain.Tenant, error) {
	id := uuid.NewString()
	owner, err := domain.NewMembership(ownerID, id, domain.RoleOwner)
	if err != nil {
		return nil, err
	}
	err = r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		err := tx.Exec(ctx, `DECLARE $id AS Utf8;
DECLARE $slug AS Utf8;
DECLARE $name AS Utf8;
INSERT INTO tenants (id, slug, name, status, created_at, updated_at)
VALUES ($id, $slug, $name, "active"u, CurrentUtcTimestamp(), CurrentUtcTimestamp());`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$id").Text(id).
				Param("$slug").Text(t.Slug().String()).
				Param("$name").Text(t.Name()).
				Build()))
		if db.IsYDBConflict(err) {
			return domain.ErrSlugTaken
		}
		if err != nil {
			return fmt.Errorf("inserting tenant: %w", err)
		}
		if err := AddYDBMembership(ctx, tx, owner); err != nil {
			return err
		}
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $display_name AS Utf8;
INSERT INTO tenant_settings (tenant_id, display_name, timezone, raw_html_sanitization, created_at, updated_at)
VALUES ($tenant_id, $display_name, "UTC"u, "standard"u, CurrentUtcTimestamp(), CurrentUtcTimestamp());`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(id).
				Param("$display_name").Text(t.Name()).
				Build())); err != nil {
			return fmt.Errorf("inserting tenant settings: %w", err)
		}
		return nil
	})
	// The spike saw the conflict at the statement; this covers a YDB that
	// reports it at commit. Only the slug index can collide: the id is fresh.
	if db.IsYDBConflict(err) {
		return nil, domain.ErrSlugTaken
	}
	if err != nil {
		return nil, err
	}
	return domain.HydrateTenant(id, t.Slug().String(), t.Name(), string(domain.StatusActive)), nil
}

// GetBySlug returns the tenant with the given slug, or
// domain.ErrTenantNotFound. Slugs are stored lower-cased (domain.NewSlug
// lower-cases them), so lower-casing the lookup matches case-insensitively,
// as the Postgres citext column does.
func (r *YDBTenants) GetBySlug(ctx context.Context, slug string) (*domain.Tenant, error) {
	return ydbTenantFromRow(r.driver.Query().QueryRow(ctx, `DECLARE $slug AS Utf8;
SELECT id, slug, name, status
FROM tenants VIEW idx_tenants_slug
WHERE slug = $slug;`,
		query.WithParameters(ydb.ParamsBuilder().Param("$slug").Text(strings.ToLower(slug)).Build())))
}

// GetByID returns the tenant with the given id, or domain.ErrTenantNotFound.
func (r *YDBTenants) GetByID(ctx context.Context, id string) (*domain.Tenant, error) {
	return ydbTenantFromRow(r.driver.Query().QueryRow(ctx, `DECLARE $id AS Utf8;
SELECT id, slug, name, status
FROM tenants
WHERE id = $id;`,
		query.WithParameters(ydb.ParamsBuilder().Param("$id").Text(id).Build())))
}

// ydbSelectMembershipRole reads one membership by its full primary key.
const ydbSelectMembershipRole = `DECLARE $tenant_id AS Utf8;
DECLARE $platform_user_id AS Utf8;
SELECT role FROM platform_user_tenants
WHERE tenant_id = $tenant_id AND platform_user_id = $platform_user_id;`

// GetMembershipRole returns the user's role in the tenant, or
// domain.ErrNotMember.
func (r *YDBTenants) GetMembershipRole(ctx context.Context, userID, tenantID string) (domain.Role, error) {
	row, err := r.driver.Query().QueryRow(ctx, ydbSelectMembershipRole, ydbMembershipKey(tenantID, userID))
	if errors.Is(err, query.ErrNoRows) {
		return domain.Role{}, domain.ErrNotMember
	}
	if err != nil {
		return domain.Role{}, fmt.Errorf("loading membership: %w", err)
	}
	var role string
	if err := row.Scan(&role); err != nil {
		return domain.Role{}, fmt.Errorf("scanning membership: %w", err)
	}
	parsed, err := domain.NewRole(role)
	if err != nil {
		return domain.Role{}, fmt.Errorf("loading membership role: %w", err)
	}
	return parsed, nil
}

// AddMembership records a membership. Re-adding an existing member is a no-op
// that keeps their role.
func (r *YDBTenants) AddMembership(ctx context.Context, m *domain.Membership) error {
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		return AddYDBMembership(ctx, tx, m)
	})
	// A concurrent add of the same membership surfaces as a serializable
	// abort, which the SDK retries. This covers a YDB that reports it as a key
	// conflict at commit instead. Either way the member exists.
	if db.IsYDBConflict(err) {
		return nil
	}
	return err
}

// AddYDBMembership records m inside tx unless the user is already a member of
// the tenant, in which case the existing role is kept. It is the YDB form of
// Postgres's INSERT ... ON CONFLICT DO NOTHING:
//   - an INSERT that hits the existing key aborts the whole transaction (spec
//     "Plan B2 planning spike");
//   - an UPSERT would overwrite an owner's role.
//
// The read and the insert share the serializable transaction. A concurrent
// insert of the same membership aborts one of the two, and the SDK retries it.
// YDBTenants and the serverless onboarding (internal/service) share it.
func AddYDBMembership(ctx context.Context, tx query.TxActor, m *domain.Membership) error {
	_, err := tx.QueryRow(ctx, ydbSelectMembershipRole, ydbMembershipKey(m.TenantID(), m.UserID()))
	if err == nil {
		return nil
	}
	if !errors.Is(err, query.ErrNoRows) {
		return fmt.Errorf("checking membership: %w", err)
	}
	if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $platform_user_id AS Utf8;
DECLARE $role AS Utf8;
INSERT INTO platform_user_tenants (tenant_id, platform_user_id, role, created_at)
VALUES ($tenant_id, $platform_user_id, $role, CurrentUtcTimestamp());`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(m.TenantID()).
			Param("$platform_user_id").Text(m.UserID()).
			Param("$role").Text(m.Role().String()).
			Build())); err != nil {
		return fmt.Errorf("inserting membership: %w", err)
	}
	return nil
}

// ListMembershipsForUser returns every tenant the user belongs to, with the
// user's role in each, oldest membership first.
func (r *YDBTenants) ListMembershipsForUser(ctx context.Context, userID string) ([]domain.MembershipDetail, error) {
	rs, err := r.driver.Query().QueryResultSet(ctx, `DECLARE $platform_user_id AS Utf8;
SELECT t.id, t.slug, t.name, t.status, m.role
FROM platform_user_tenants VIEW idx_platform_user_tenants_user AS m
JOIN tenants AS t ON t.id = m.tenant_id
WHERE m.platform_user_id = $platform_user_id
ORDER BY m.created_at;`,
		query.WithParameters(ydb.ParamsBuilder().Param("$platform_user_id").Text(userID).Build()))
	if err != nil {
		return nil, fmt.Errorf("listing memberships: %w", err)
	}
	defer func() { _ = rs.Close(ctx) }()
	memberships := []domain.MembershipDetail{}
	for row, err := range rs.Rows(ctx) {
		if err != nil {
			return nil, fmt.Errorf("listing memberships: %w", err)
		}
		var id, slug, name, status, role string
		if err := row.Scan(&id, &slug, &name, &status, &role); err != nil {
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
	return memberships, nil
}

// ListMembers returns every member of the tenant, oldest membership first. It
// joins platform_users, which the auth context owns, for the email and name.
func (r *YDBTenants) ListMembers(ctx context.Context, tenantID string) ([]domain.Member, error) {
	rs, err := r.driver.Query().QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
SELECT u.id, u.email, u.name, m.role
FROM platform_user_tenants AS m
JOIN platform_users AS u ON u.id = m.platform_user_id
WHERE m.tenant_id = $tenant_id
ORDER BY m.created_at;`, ydbTenantParam(tenantID))
	if err != nil {
		return nil, fmt.Errorf("listing members: %w", err)
	}
	defer func() { _ = rs.Close(ctx) }()
	members := []domain.Member{}
	for row, err := range rs.Rows(ctx) {
		if err != nil {
			return nil, fmt.Errorf("listing members: %w", err)
		}
		var userID, email, name, role string
		if err := row.Scan(&userID, &email, &name, &role); err != nil {
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
	return members, nil
}

// ydbTenantParam binds the $tenant_id parameter.
func ydbTenantParam(tenantID string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().Param("$tenant_id").Text(tenantID).Build())
}

// ydbMembershipKey binds the platform_user_tenants primary key.
func ydbMembershipKey(tenantID, userID string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$platform_user_id").Text(userID).
		Build())
}

// ydbTenantFromRow maps a single-row read of (id, slug, name, status) to the
// tenant, translating "no row" into domain.ErrTenantNotFound.
func ydbTenantFromRow(row query.Row, err error) (*domain.Tenant, error) {
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrTenantNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading tenant: %w", err)
	}
	var id, slug, name, status string
	if err := row.Scan(&id, &slug, &name, &status); err != nil {
		return nil, fmt.Errorf("scanning tenant: %w", err)
	}
	return domain.HydrateTenant(id, slug, name, status), nil
}

package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/db"
	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

// YDBRoles is the YDB-backed implementation of domain.RoleRepository, for the
// serverless runtime. A role's permissions are stored as a JSON array in a
// Utf8 column. YDB has no foreign keys, so an assignment checks that its role
// exists in the same transaction that writes it.
type YDBRoles struct {
	driver *ydb.Driver
}

var _ domain.RoleRepository = (*YDBRoles)(nil)

// NewYDBRoles builds a YDBRoles repository over the driver.
func NewYDBRoles(d *ydb.Driver) *YDBRoles {
	return &YDBRoles{driver: d}
}

// ydbRoleColumns is the projection ydbRoleFromRow reads, in order.
const ydbRoleColumns = `id, tenant_id, name, permissions, created_at, updated_at`

const ydbSelectRole = `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT ` + ydbRoleColumns + `
FROM roles
WHERE tenant_id = $tenant_id AND id = $id;`

// Add persists a new role under a fresh UUID and returns the id. It returns
// domain.ErrRoleNameTaken when the tenant already has a role with that name.
func (r *YDBRoles) Add(ctx context.Context, tenantID string, role *domain.Role) (string, error) {
	id := uuid.NewString()
	err := r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $name AS Utf8;
DECLARE $permissions AS Utf8;
INSERT INTO roles (tenant_id, id, name, permissions, created_at, updated_at)
VALUES ($tenant_id, $id, $name, $permissions, CurrentUtcTimestamp(), CurrentUtcTimestamp());`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$id").Text(id).
			Param("$name").Text(role.Name()).
			Param("$permissions").Text(ydbPermissions(role.Permissions())).
			Build()))
	// Only the name index can collide: the id is fresh.
	if db.IsYDBConflict(err) {
		return "", domain.ErrRoleNameTaken
	}
	if err != nil {
		return "", fmt.Errorf("inserting role: %w", err)
	}
	return id, nil
}

// Update loads the role, runs fn, and persists the result in one serializable
// transaction. It returns domain.ErrRoleNameTaken when the new name belongs to
// another role of the tenant. The SDK retries the transaction on conflict, so
// fn may run more than once and must be retry-safe.
func (r *YDBRoles) Update(ctx context.Context, tenantID, id string,
	fn func(*domain.Role) (*domain.Role, error)) error {

	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		loaded, err := ydbRoleFromRow(tx.QueryRow(ctx, ydbSelectRole, ydbTenantAndID(tenantID, id)))
		if err != nil {
			return err
		}
		updated, err := fn(loaded)
		if err != nil {
			return err
		}
		err = tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $name AS Utf8;
DECLARE $permissions AS Utf8;
UPDATE roles SET name = $name, permissions = $permissions, updated_at = CurrentUtcTimestamp()
WHERE tenant_id = $tenant_id AND id = $id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$id").Text(id).
				Param("$name").Text(updated.Name()).
				Param("$permissions").Text(ydbPermissions(updated.Permissions())).
				Build()))
		// An UPDATE onto another role's name hits the unique name index at
		// the statement (spec "Plan B4 planning spike").
		if db.IsYDBConflict(err) {
			return domain.ErrRoleNameTaken
		}
		if err != nil {
			return fmt.Errorf("updating role: %w", err)
		}
		return nil
	})
	// This covers a YDB that reports the collision at commit.
	if db.IsYDBConflict(err) {
		return domain.ErrRoleNameTaken
	}
	return err
}

// Delete removes the role. It returns domain.ErrRoleInUse when the role is
// still assigned, tenant-wide or per list, and domain.ErrRoleNotFound when the
// tenant has no such role. A concurrent assignment of the role aborts one of
// the two transactions, and the SDK retries it.
func (r *YDBRoles) Delete(ctx context.Context, tenantID, id string) error {
	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		// ponytail: each check scans the tenant's assignments (no role_id
		// index); add GLOBAL SYNC indexes on (tenant_id, role_id) if a tenant
		// has many users.
		for _, table := range []string{"user_roles", "user_list_roles"} {
			_, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $role_id AS Utf8;
SELECT role_id FROM `+table+`
WHERE tenant_id = $tenant_id AND role_id = $role_id
LIMIT 1;`,
				query.WithParameters(ydb.ParamsBuilder().
					Param("$tenant_id").Text(tenantID).
					Param("$role_id").Text(id).
					Build()))
			if err == nil {
				return domain.ErrRoleInUse
			}
			if !errors.Is(err, query.ErrNoRows) {
				return fmt.Errorf("checking role assignments: %w", err)
			}
		}
		_, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DELETE FROM roles
WHERE tenant_id = $tenant_id AND id = $id
RETURNING id;`, ydbTenantAndID(tenantID, id))
		if errors.Is(err, query.ErrNoRows) {
			return domain.ErrRoleNotFound
		}
		if err != nil {
			return fmt.Errorf("deleting role: %w", err)
		}
		return nil
	})
}

// Get returns the role, or domain.ErrRoleNotFound.
func (r *YDBRoles) Get(ctx context.Context, tenantID, id string) (*domain.Role, error) {
	return ydbRoleFromRow(r.driver.Query().QueryRow(ctx, ydbSelectRole, ydbTenantAndID(tenantID, id)))
}

// All returns every role in the tenant, ordered by name.
func (r *YDBRoles) All(ctx context.Context, tenantID string) ([]*domain.Role, error) {
	rs, err := r.driver.Query().QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
SELECT `+ydbRoleColumns+`
FROM roles
WHERE tenant_id = $tenant_id
ORDER BY name;`, ydbTenantParam(tenantID))
	if err != nil {
		return nil, fmt.Errorf("listing roles: %w", err)
	}
	defer func() { _ = rs.Close(ctx) }()
	var roles []*domain.Role
	for row, err := range rs.Rows(ctx) {
		if err != nil {
			return nil, fmt.Errorf("listing roles: %w", err)
		}
		role, err := scanYDBRole(row)
		if err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, nil
}

// AssignTenantRole sets a user's tenant-level role, replacing any previous
// one. It returns domain.ErrRoleNotFound when the tenant has no such role,
// standing in for the Postgres foreign key.
func (r *YDBRoles) AssignTenantRole(ctx context.Context, tenantID, userID, roleID string) error {
	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		if err := ydbRoleExists(ctx, tx, tenantID, roleID); err != nil {
			return err
		}
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $user_id AS Utf8;
DECLARE $role_id AS Utf8;
UPSERT INTO user_roles (tenant_id, user_id, role_id)
VALUES ($tenant_id, $user_id, $role_id);`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$user_id").Text(userID).
				Param("$role_id").Text(roleID).
				Build())); err != nil {
			return fmt.Errorf("assigning tenant role: %w", err)
		}
		return nil
	})
}

// AssignListRole grants a user a per-list role, replacing any previous role
// on that list. It returns domain.ErrRoleNotFound when the tenant has no such
// role, standing in for the Postgres foreign key. The list is not checked: the
// lists table belongs to the audience context.
func (r *YDBRoles) AssignListRole(ctx context.Context, tenantID, userID, listID, roleID string) error {
	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		if err := ydbRoleExists(ctx, tx, tenantID, roleID); err != nil {
			return err
		}
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $user_id AS Utf8;
DECLARE $list_id AS Utf8;
DECLARE $role_id AS Utf8;
UPSERT INTO user_list_roles (tenant_id, user_id, list_id, role_id)
VALUES ($tenant_id, $user_id, $list_id, $role_id);`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$user_id").Text(userID).
				Param("$list_id").Text(listID).
				Param("$role_id").Text(roleID).
				Build())); err != nil {
			return fmt.Errorf("assigning list role: %w", err)
		}
		return nil
	})
}

// RemoveListRole removes a user's per-list role. Removing a role the user does
// not hold is a no-op.
func (r *YDBRoles) RemoveListRole(ctx context.Context, tenantID, userID, listID string) error {
	if err := r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $user_id AS Utf8;
DECLARE $list_id AS Utf8;
DELETE FROM user_list_roles
WHERE tenant_id = $tenant_id AND user_id = $user_id AND list_id = $list_id;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$user_id").Text(userID).
			Param("$list_id").Text(listID).
			Build())); err != nil {
		return fmt.Errorf("removing list role: %w", err)
	}
	return nil
}

// EffectiveFor loads a user's tenant-level permissions and the permissions of
// each per-list role they hold, in one transaction. The joins match the role
// on (tenant_id, id), so another tenant's role never contributes.
func (r *YDBRoles) EffectiveFor(ctx context.Context, tenantID, userID string) (
	[]domain.Permission, map[string][]domain.Permission, error) {

	params := query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$user_id").Text(userID).
		Build())
	var tenantPerms []domain.Permission
	var listPerms map[string][]domain.Permission
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		// The user has at most one tenant-level role. No row means none.
		tenantLevel := permsFromStrings(nil)
		row, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $user_id AS Utf8;
SELECT r.permissions
FROM user_roles AS ur
JOIN roles AS r ON r.tenant_id = ur.tenant_id AND r.id = ur.role_id
WHERE ur.tenant_id = $tenant_id AND ur.user_id = $user_id;`, params)
		switch {
		case errors.Is(err, query.ErrNoRows):
		case err != nil:
			return fmt.Errorf("loading tenant role: %w", err)
		default:
			var raw string
			if err := row.Scan(&raw); err != nil {
				return fmt.Errorf("scanning tenant role: %w", err)
			}
			if tenantLevel, err = ydbPermissionsFrom(raw); err != nil {
				return err
			}
		}

		rs, err := tx.QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $user_id AS Utf8;
SELECT ulr.list_id, r.permissions
FROM user_list_roles AS ulr
JOIN roles AS r ON r.tenant_id = ulr.tenant_id AND r.id = ulr.role_id
WHERE ulr.tenant_id = $tenant_id AND ulr.user_id = $user_id;`, params)
		if err != nil {
			return fmt.Errorf("loading list roles: %w", err)
		}
		// Built locally and assigned at the end: the SDK may re-run the
		// closure, and a retry must start from an empty map.
		perList := map[string][]domain.Permission{}
		for row, err := range rs.Rows(ctx) {
			if err != nil {
				return fmt.Errorf("loading list roles: %w", err)
			}
			var listID, raw string
			if err := row.Scan(&listID, &raw); err != nil {
				return fmt.Errorf("scanning list role: %w", err)
			}
			perms, err := ydbPermissionsFrom(raw)
			if err != nil {
				return err
			}
			perList[listID] = perms
		}
		tenantPerms, listPerms = tenantLevel, perList
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return tenantPerms, listPerms, nil
}

// ydbRoleExists returns domain.ErrRoleNotFound unless the tenant has the role.
// Reading the role inside the assigning transaction makes a concurrent Delete
// of it abort one of the two, as the Postgres foreign key would.
func ydbRoleExists(ctx context.Context, tx query.TxActor, tenantID, roleID string) error {
	_, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT id FROM roles
WHERE tenant_id = $tenant_id AND id = $id;`, ydbTenantAndID(tenantID, roleID))
	if errors.Is(err, query.ErrNoRows) {
		return domain.ErrRoleNotFound
	}
	if err != nil {
		return fmt.Errorf("checking role: %w", err)
	}
	return nil
}

// ydbPermissions encodes a permission set as the JSON array the permissions
// columns store.
func ydbPermissions(perms []domain.Permission) string {
	raw, _ := json.Marshal(permStrings(perms)) // a []string always encodes
	return string(raw)
}

// ydbPermissionsFrom decodes a permissions column.
func ydbPermissionsFrom(raw string) ([]domain.Permission, error) {
	var perms []string
	if err := json.Unmarshal([]byte(raw), &perms); err != nil {
		return nil, fmt.Errorf("decoding permissions: %w", err)
	}
	return permsFromStrings(perms), nil
}

// ydbRoleFromRow maps a single-row read to the role, translating "no row"
// into domain.ErrRoleNotFound.
func ydbRoleFromRow(row query.Row, err error) (*domain.Role, error) {
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrRoleNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading role: %w", err)
	}
	return scanYDBRole(row)
}

// scanYDBRole reads one row in ydbRoleColumns order.
func scanYDBRole(row query.Row) (*domain.Role, error) {
	var id, tenantID, name, raw string
	var createdAt, updatedAt time.Time
	if err := row.Scan(&id, &tenantID, &name, &raw, &createdAt, &updatedAt); err != nil {
		return nil, fmt.Errorf("scanning role: %w", err)
	}
	perms, err := ydbPermissionsFrom(raw)
	if err != nil {
		return nil, err
	}
	return domain.HydrateRole(id, tenantID, name, perms, createdAt, updatedAt), nil
}

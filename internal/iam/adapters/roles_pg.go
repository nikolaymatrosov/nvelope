// Package adapters implements the iam domain's repository and capability
// interfaces against PostgreSQL. Every tenant-plane operation runs inside the
// shared RLS-bound transaction (internal/platform/tenantdb).
package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/db"
	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// Roles is the pgx-backed implementation of domain.RoleRepository.
type Roles struct {
	pool *pgxpool.Pool
}

var _ domain.RoleRepository = (*Roles)(nil)

// NewRoles builds a Roles repository over the given pool.
func NewRoles(pool *pgxpool.Pool) *Roles {
	return &Roles{pool: pool}
}

// permStrings converts a permission slice to the []string the text[] column
// stores.
func permStrings(perms []domain.Permission) []string {
	out := make([]string, len(perms))
	for i, p := range perms {
		out[i] = string(p)
	}
	return out
}

// permsFromStrings converts a stored []string back to a permission slice.
func permsFromStrings(raw []string) []domain.Permission {
	out := make([]domain.Permission, len(raw))
	for i, s := range raw {
		out[i] = domain.Permission(s)
	}
	return out
}

// Add persists a new role and returns its database-assigned id.
func (r *Roles) Add(ctx context.Context, tenantID string, role *domain.Role) (string, error) {
	var id string
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			"INSERT INTO roles (tenant_id, name, permissions) VALUES ($1, $2, $3) RETURNING id",
			tenantID, role.Name(), permStrings(role.Permissions())).Scan(&id)
		if db.IsUniqueViolation(err) {
			return domain.ErrRoleNameTaken
		}
		if err != nil {
			return fmt.Errorf("inserting role: %w", err)
		}
		return nil
	})
	return id, err
}

// Update loads the role, runs fn, and persists the result.
func (r *Roles) Update(ctx context.Context, tenantID, id string,
	fn func(*domain.Role) (*domain.Role, error)) error {

	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		role, err := getRoleTx(ctx, tx, id)
		if err != nil {
			return err
		}
		updated, err := fn(role)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx,
			"UPDATE roles SET name = $1, permissions = $2, updated_at = now() WHERE id = $3",
			updated.Name(), permStrings(updated.Permissions()), id)
		if db.IsUniqueViolation(err) {
			return domain.ErrRoleNameTaken
		}
		if err != nil {
			return fmt.Errorf("updating role: %w", err)
		}
		return nil
	})
}

// Delete removes the role, rejecting the deletion when it is still assigned.
func (r *Roles) Delete(ctx context.Context, tenantID, id string) error {
	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		var assigned int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM (
			   SELECT role_id FROM user_roles WHERE role_id = $1
			   UNION ALL
			   SELECT role_id FROM user_list_roles WHERE role_id = $1
			 ) a`, id).Scan(&assigned); err != nil {
			if db.IsInvalidInput(err) {
				return domain.ErrRoleNotFound
			}
			return fmt.Errorf("checking role assignments: %w", err)
		}
		if assigned > 0 {
			return domain.ErrRoleInUse
		}
		tag, err := tx.Exec(ctx, "DELETE FROM roles WHERE id = $1", id)
		if err != nil {
			return fmt.Errorf("deleting role: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrRoleNotFound
		}
		return nil
	})
}

// Get returns the role, or domain.ErrRoleNotFound.
func (r *Roles) Get(ctx context.Context, tenantID, id string) (*domain.Role, error) {
	var out *domain.Role
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		role, err := getRoleTx(ctx, tx, id)
		if err != nil {
			return err
		}
		out = role
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// All returns every role in the tenant.
func (r *Roles) All(ctx context.Context, tenantID string) ([]*domain.Role, error) {
	var out []*domain.Role
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			"SELECT id, tenant_id, name, permissions, created_at, updated_at FROM roles ORDER BY name")
		if err != nil {
			return fmt.Errorf("listing roles: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			role, err := scanRole(rows)
			if err != nil {
				return err
			}
			out = append(out, role)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// AssignTenantRole sets a user's tenant-level role.
func (r *Roles) AssignTenantRole(ctx context.Context, tenantID, userID, roleID string) error {
	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO user_roles (tenant_id, user_id, role_id) VALUES ($1, $2, $3)
			 ON CONFLICT (tenant_id, user_id) DO UPDATE SET role_id = EXCLUDED.role_id`,
			tenantID, userID, roleID)
		if db.IsInvalidInput(err) {
			return domain.ErrRoleNotFound
		}
		if err != nil {
			return fmt.Errorf("assigning tenant role: %w", err)
		}
		return nil
	})
}

// AssignListRole grants a user a per-list role.
func (r *Roles) AssignListRole(ctx context.Context, tenantID, userID, listID, roleID string) error {
	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO user_list_roles (tenant_id, user_id, list_id, role_id)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (tenant_id, user_id, list_id) DO UPDATE SET role_id = EXCLUDED.role_id`,
			tenantID, userID, listID, roleID)
		if db.IsInvalidInput(err) {
			return domain.ErrRoleNotFound
		}
		if err != nil {
			return fmt.Errorf("assigning list role: %w", err)
		}
		return nil
	})
}

// RemoveListRole removes a user's per-list role.
func (r *Roles) RemoveListRole(ctx context.Context, tenantID, userID, listID string) error {
	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			"DELETE FROM user_list_roles WHERE user_id = $1 AND list_id = $2", userID, listID)
		if err != nil && !db.IsInvalidInput(err) {
			return fmt.Errorf("removing list role: %w", err)
		}
		return nil
	})
}

// EffectiveFor loads a user's tenant-level permissions and per-list role
// permissions in one tenant-bound transaction.
func (r *Roles) EffectiveFor(ctx context.Context, tenantID, userID string) (
	[]domain.Permission, map[string][]domain.Permission, error) {

	var tenantPerms []domain.Permission
	listPerms := map[string][]domain.Permission{}
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		var raw []string
		err := tx.QueryRow(ctx,
			`SELECT r.permissions FROM user_roles ur
			 JOIN roles r ON r.id = ur.role_id WHERE ur.user_id = $1`, userID).Scan(&raw)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			if db.IsInvalidInput(err) {
				return nil
			}
			return fmt.Errorf("loading tenant role: %w", err)
		}
		tenantPerms = permsFromStrings(raw)

		rows, err := tx.Query(ctx,
			`SELECT ulr.list_id, r.permissions FROM user_list_roles ulr
			 JOIN roles r ON r.id = ulr.role_id WHERE ulr.user_id = $1`, userID)
		if err != nil {
			return fmt.Errorf("loading list roles: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var listID string
			var perms []string
			if err := rows.Scan(&listID, &perms); err != nil {
				return fmt.Errorf("scanning list role: %w", err)
			}
			listPerms[listID] = permsFromStrings(perms)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, nil, err
	}
	return tenantPerms, listPerms, nil
}

func getRoleTx(ctx context.Context, tx pgx.Tx, id string) (*domain.Role, error) {
	row := tx.QueryRow(ctx,
		"SELECT id, tenant_id, name, permissions, created_at, updated_at FROM roles WHERE id = $1", id)
	role, err := scanRole(row)
	if errors.Is(err, pgx.ErrNoRows) || db.IsInvalidInput(err) {
		return nil, domain.ErrRoleNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading role: %w", err)
	}
	return role, nil
}

func scanRole(row pgx.Row) (*domain.Role, error) {
	var id, tenantID, name string
	var perms []string
	var createdAt, updatedAt time.Time
	if err := row.Scan(&id, &tenantID, &name, &perms, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	return domain.HydrateRole(id, tenantID, name, permsFromStrings(perms), createdAt, updatedAt), nil
}

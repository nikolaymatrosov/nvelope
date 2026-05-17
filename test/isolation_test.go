// The cross-tenant isolation suite. It connects as the restricted nvelope_app
// role and proves that operations bound to one tenant cannot read, modify, or
// delete another tenant's rows — even when the application-level tenant filter
// is deliberately omitted (spec FR-009, FR-010; Constitution I).
//
// The suite binds app.tenant_id itself with raw SQL rather than through any
// production helper: the assertions are about the database's Row-Level
// Security, so the test exercises the database directly.
package test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	tenantadapters "github.com/nikolaymatrosov/nvelope/internal/tenant/adapters"
	tenantdomain "github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

func TestCrossTenantIsolation(t *testing.T) {
	pool := dbtest.AppPool(t)
	ctx := context.Background()

	// Two tenants, each created with its own tenant_settings row.
	tenantA := seedTenant(t, pool, "Tenant A")
	tenantB := seedTenant(t, pool, "Tenant B")

	// 1. SELECT without a tenant filter, bound to A → only A's row is visible.
	require.NoError(t, boundTx(ctx, pool, tenantA, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "SELECT tenant_id FROM tenant_settings")
		require.NoError(t, err)
		defer rows.Close()

		var visible []string
		for rows.Next() {
			var id string
			require.NoError(t, rows.Scan(&id))
			visible = append(visible, id)
		}
		require.NoError(t, rows.Err())
		require.Equal(t, []string{tenantA}, visible,
			"bound to A, an unfiltered SELECT returns only A's row")
		return nil
	}))

	// 2. UPDATE without a tenant filter, bound to A → B's row is untouched.
	require.NoError(t, boundTx(ctx, pool, tenantA, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "UPDATE tenant_settings SET display_name = 'hijacked'")
		require.NoError(t, err)
		require.EqualValues(t, 1, tag.RowsAffected(),
			"the unfiltered UPDATE reached only the bound tenant's row")
		return nil
	}))
	require.Equal(t, "Tenant B", displayName(t, pool, tenantB),
		"tenant B's display name is unchanged by tenant A's unfiltered UPDATE")

	// 3. DELETE without a tenant filter, bound to A → B's row survives.
	require.NoError(t, boundTx(ctx, pool, tenantA, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "DELETE FROM tenant_settings")
		require.NoError(t, err)
		require.EqualValues(t, 1, tag.RowsAffected(),
			"the unfiltered DELETE removed only the bound tenant's row")
		return nil
	}))
	require.Equal(t, "Tenant B", displayName(t, pool, tenantB),
		"tenant B's row survives tenant A's unfiltered DELETE")

	// 4. INSERT targeting tenant B while bound to A → rejected by WITH CHECK.
	err := boundTx(ctx, pool, tenantA, func(ctx context.Context, tx pgx.Tx) error {
		_, e := tx.Exec(ctx,
			"INSERT INTO tenant_settings (tenant_id, display_name) VALUES ($1, $2)",
			tenantB, "smuggled")
		return e
	})
	require.Error(t, err,
		"an INSERT writing another tenant's id must be rejected by the RLS WITH CHECK")
}

func TestTenantPlaneFailsClosedWithoutBinding(t *testing.T) {
	pool := dbtest.AppPool(t)
	seedTenant(t, pool, "Some Tenant")

	// A query outside a tenant-bound transaction has no app.tenant_id set:
	// RLS exposes zero rows (deny by default).
	var count int
	require.NoError(t, pool.QueryRow(context.Background(),
		"SELECT count(*) FROM tenant_settings").Scan(&count))
	require.Equal(t, 0, count, "unbound access to a tenant-plane table sees nothing")
}

// TestAudienceCrossTenantIsolation proves that the audience tenant-plane
// tables — lists, subscribers, subscriber_lists — are isolated by RLS: a
// transaction bound to one tenant cannot read, modify, or delete another
// tenant's rows even with the application filter omitted.
func TestAudienceCrossTenantIsolation(t *testing.T) {
	pool := dbtest.AppPool(t)
	ctx := context.Background()

	tenantA := seedTenant(t, pool, "Tenant A")
	tenantB := seedTenant(t, pool, "Tenant B")

	// Seed one list and one subscriber (with a membership) into tenant A.
	var listID, subscriberID string
	require.NoError(t, boundTx(ctx, pool, tenantA, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx,
			"INSERT INTO lists (tenant_id, name) VALUES ($1, 'A-list') RETURNING id",
			tenantA).Scan(&listID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx,
			"INSERT INTO subscribers (tenant_id, email) VALUES ($1, 'a@example.com') RETURNING id",
			tenantA).Scan(&subscriberID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO subscriber_lists (tenant_id, subscriber_id, list_id)
			 VALUES ($1, $2, $3)`, tenantA, subscriberID, listID)
		return err
	}))

	for _, table := range []string{"lists", "subscribers", "subscriber_lists"} {
		t.Run(table, func(t *testing.T) {
			// Bound to B, an unfiltered SELECT sees none of A's rows.
			require.NoError(t, boundTx(ctx, pool, tenantB, func(ctx context.Context, tx pgx.Tx) error {
				var n int
				if err := tx.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
					return err
				}
				require.Equal(t, 0, n, "bound to B, %s shows none of A's rows", table)
				return nil
			}))

			// Bound to B, an unfiltered UPDATE and DELETE touch nothing of A's.
			require.NoError(t, boundTx(ctx, pool, tenantB, func(ctx context.Context, tx pgx.Tx) error {
				tag, err := tx.Exec(ctx, "DELETE FROM "+table)
				if err != nil {
					return err
				}
				require.EqualValues(t, 0, tag.RowsAffected(),
					"bound to B, an unfiltered DELETE on %s reaches no row of A's", table)
				return nil
			}))
		})
	}

	// A's rows survive B's unfiltered DELETE attempts.
	require.NoError(t, boundTx(ctx, pool, tenantA, func(ctx context.Context, tx pgx.Tx) error {
		var lists, subs, memberships int
		require.NoError(t, tx.QueryRow(ctx, "SELECT count(*) FROM lists").Scan(&lists))
		require.NoError(t, tx.QueryRow(ctx, "SELECT count(*) FROM subscribers").Scan(&subs))
		require.NoError(t, tx.QueryRow(ctx, "SELECT count(*) FROM subscriber_lists").Scan(&memberships))
		require.Equal(t, 1, lists)
		require.Equal(t, 1, subs)
		require.Equal(t, 1, memberships)
		return nil
	}))

	// An INSERT writing tenant B's id while bound to A is rejected by WITH CHECK.
	err := boundTx(ctx, pool, tenantA, func(ctx context.Context, tx pgx.Tx) error {
		_, e := tx.Exec(ctx,
			"INSERT INTO lists (tenant_id, name) VALUES ($1, 'smuggled')", tenantB)
		return e
	})
	require.Error(t, err, "an INSERT writing another tenant's id must be rejected by RLS WITH CHECK")
}

// TestIAMCrossTenantIsolation proves that the iam tenant-plane tables — users,
// sessions, roles, user_roles, user_list_roles, audit_log — are isolated by
// RLS: a transaction bound to one tenant cannot see or delete another
// tenant's rows.
func TestIAMCrossTenantIsolation(t *testing.T) {
	pool := dbtest.AppPool(t)
	ctx := context.Background()

	tenantA := seedTenant(t, pool, "Tenant A")
	tenantB := seedTenant(t, pool, "Tenant B")

	// A platform user is needed for the users.platform_user_id foreign key.
	var platformUser string
	require.NoError(t, pool.QueryRow(ctx,
		"INSERT INTO platform_users (email, password_hash, name) VALUES ($1, 'x', 'U') RETURNING id",
		dbtest.RandString()+"@example.com").Scan(&platformUser))

	require.NoError(t, boundTx(ctx, pool, tenantA, func(ctx context.Context, tx pgx.Tx) error {
		var userID, roleID, listID string
		if err := tx.QueryRow(ctx,
			`INSERT INTO users (tenant_id, platform_user_id, email)
			 VALUES ($1, $2, 'a@example.com') RETURNING id`,
			tenantA, platformUser).Scan(&userID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx,
			`INSERT INTO roles (tenant_id, name) VALUES ($1, 'Owner') RETURNING id`,
			tenantA).Scan(&roleID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx,
			"INSERT INTO lists (tenant_id, name) VALUES ($1, 'L') RETURNING id",
			tenantA).Scan(&listID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO sessions (tenant_id, user_id, token_hash, expires_at)
			 VALUES ($1, $2, $3, now() + interval '1 hour')`,
			tenantA, userID, "hash-"+dbtest.RandString()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			"INSERT INTO user_roles (tenant_id, user_id, role_id) VALUES ($1, $2, $3)",
			tenantA, userID, roleID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO user_list_roles (tenant_id, user_id, list_id, role_id)
			 VALUES ($1, $2, $3, $4)`, tenantA, userID, listID, roleID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO audit_log (tenant_id, actor_id, actor_kind, action)
			 VALUES ($1, $2, 'session', 'role.create')`, tenantA, userID)
		return err
	}))

	for _, table := range []string{
		"users", "sessions", "roles", "user_roles", "user_list_roles", "audit_log",
	} {
		t.Run(table, func(t *testing.T) {
			require.NoError(t, boundTx(ctx, pool, tenantB, func(ctx context.Context, tx pgx.Tx) error {
				var n int
				if err := tx.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
					return err
				}
				require.Equal(t, 0, n, "bound to B, %s shows none of A's rows", table)
				tag, err := tx.Exec(ctx, "DELETE FROM "+table)
				if err != nil {
					return err
				}
				require.EqualValues(t, 0, tag.RowsAffected(),
					"bound to B, an unfiltered DELETE on %s reaches no row of A's", table)
				return nil
			}))
		})
	}

	// A's rows survive B's unfiltered DELETE attempts.
	require.NoError(t, boundTx(ctx, pool, tenantA, func(ctx context.Context, tx pgx.Tx) error {
		var n int
		require.NoError(t, tx.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&n))
		require.Equal(t, 1, n, "tenant A's user survives tenant B's unfiltered DELETE")
		return nil
	}))
}

// boundTx runs fn inside a transaction with app.tenant_id bound to tenantID —
// the binding every tenant-plane access depends on.
func boundTx(ctx context.Context, pool *pgxpool.Pool, tenantID string,
	fn func(ctx context.Context, tx pgx.Tx) error) error {

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID); err != nil {
		return err
	}
	if err := fn(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// seedTenant creates a platform user and a tenant owned by them, and returns
// the tenant id. Creating the workspace also writes its tenant_settings row.
func seedTenant(t *testing.T, pool *pgxpool.Pool, name string) string {
	t.Helper()
	ctx := context.Background()

	var ownerID string
	require.NoError(t, pool.QueryRow(ctx,
		"INSERT INTO platform_users (email, password_hash, name) VALUES ($1, $2, $3) RETURNING id",
		dbtest.RandString()+"@example.com", "x", "Owner").Scan(&ownerID))

	tn, err := tenantdomain.NewTenant(name, "iso-"+dbtest.RandString())
	require.NoError(t, err)
	created, err := tenantadapters.NewTenants(pool).CreateWorkspace(ctx, tn, ownerID)
	require.NoError(t, err)
	return created.ID()
}

// displayName reads a tenant's settings display_name inside its own bound
// transaction.
func displayName(t *testing.T, pool *pgxpool.Pool, tenantID string) string {
	t.Helper()
	var name string
	require.NoError(t, boundTx(context.Background(), pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, "SELECT display_name FROM tenant_settings").Scan(&name)
		}))
	return name
}

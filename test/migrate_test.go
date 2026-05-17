// Package test holds cross-cutting integration tests. The migration test runs
// a destructive apply/revert cycle against a freshly created scratch database
// (see dbtest.ScratchDatabaseDSN) so it never disturbs the shared test
// database. It is skipped when no database DSN is configured.
package test

import (
	"context"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/db"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

func TestMigrationsRoundTrip(t *testing.T) {
	dsn := dbtest.ScratchDatabaseDSN(t)

	newMigrator := func() *migrate.Migrate {
		src, err := iofs.New(db.MigrationsFS, "migrations")
		require.NoError(t, err)
		m, err := migrate.NewWithSourceInstance("iofs", src, dsn)
		require.NoError(t, err)
		return m
	}
	closeMigrator := func(m *migrate.Migrate) {
		srcErr, dbErr := m.Close()
		require.NoError(t, srcErr)
		require.NoError(t, dbErr)
	}

	// Apply: every migration applies and the recorded version reaches 7.
	m := newMigrator()
	require.NoError(t, m.Up())
	v, dirty, err := m.Version()
	require.NoError(t, err)
	require.Equal(t, uint(7), v)
	require.False(t, dirty)
	closeMigrator(m)

	require.True(t, extensionExists(t, dsn, "pgcrypto"), "pgcrypto should exist after up")
	require.True(t, extensionExists(t, dsn, "citext"), "citext should exist after up")
	for _, table := range []string{
		"platform_users", "platform_sessions", "tenants",
		"platform_user_tenants", "invitations", "tenant_settings",
		"users", "sessions", "roles", "user_roles", "user_list_roles",
		"api_keys", "recovery_codes", "audit_log",
		"lists", "subscribers", "subscriber_lists", "import_export_jobs",
	} {
		require.True(t, tableExists(t, dsn, table), "%s should exist after up", table)
	}
	for _, table := range []string{
		"tenant_settings", "users", "sessions", "roles", "lists", "subscribers", "subscriber_lists",
	} {
		require.True(t, rlsForced(t, dsn, table),
			"%s must have FORCE ROW LEVEL SECURITY", table)
	}

	// Idempotent: applying again is a no-op.
	m = newMigrator()
	require.ErrorIs(t, m.Up(), migrate.ErrNoChange)
	closeMigrator(m)

	// Revert: the database returns to its original empty state.
	m = newMigrator()
	require.NoError(t, m.Down())
	_, _, err = m.Version()
	require.ErrorIs(t, err, migrate.ErrNilVersion)
	closeMigrator(m)
	require.False(t, tableExists(t, dsn, "platform_users"), "tables should be gone after down")
	require.False(t, extensionExists(t, dsn, "pgcrypto"), "pgcrypto should be gone after down")
	require.False(t, extensionExists(t, dsn, "citext"), "citext should be gone after down")

	// Re-apply cleanly so the round trip is proven repeatable.
	m = newMigrator()
	require.NoError(t, m.Up())
	closeMigrator(m)
}

func extensionExists(t *testing.T, dsn, name string) bool {
	t.Helper()
	return queryBool(t, dsn,
		"SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = $1)", name)
}

func tableExists(t *testing.T, dsn, name string) bool {
	t.Helper()
	return queryBool(t, dsn,
		"SELECT EXISTS (SELECT 1 FROM information_schema.tables "+
			"WHERE table_schema = 'public' AND table_name = $1)", name)
}

func rlsForced(t *testing.T, dsn, name string) bool {
	t.Helper()
	return queryBool(t, dsn,
		"SELECT relrowsecurity AND relforcerowsecurity FROM pg_class WHERE relname = $1", name)
}

func queryBool(t *testing.T, dsn, sql string, args ...any) bool {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	require.NoError(t, err)
	defer func() { _ = conn.Close(ctx) }()

	var result bool
	require.NoError(t, conn.QueryRow(ctx, sql, args...).Scan(&result))
	return result
}

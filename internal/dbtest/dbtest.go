// Package dbtest provides shared helpers for integration tests that need a
// real PostgreSQL database. Tests calling these helpers are skipped when no
// database DSN is configured, so a database-less run still passes the unit
// suite.
package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nvelope/nvelope/internal/db"
)

// RandString returns a short random lowercase-hex string, useful for unique
// test identifiers such as emails and tenant slugs.
func RandString() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// AdminDSN returns the privileged PostgreSQL DSN used to run migrations,
// preferring NVELOPE_MIGRATE_DATABASE_URL and falling back to
// NVELOPE_DATABASE_URL. The test is skipped when neither is set.
func AdminDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("NVELOPE_MIGRATE_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("NVELOPE_DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("no database DSN configured; skipping integration test")
	}
	return dsn
}

// AppDSN returns the DSN for the restricted nvelope_app role, derived from
// AdminDSN by swapping the connection credentials. The nvelope_app role is
// created by migration 000002 with this dev-default password.
func AppDSN(t *testing.T) string {
	t.Helper()
	u, err := url.Parse(AdminDSN(t))
	require.NoError(t, err)
	u.User = url.UserPassword("nvelope_app", "nvelope_app")
	return u.String()
}

// EnsureMigrated applies every pending migration to the admin database. It is
// idempotent — safe to call from every integration test.
func EnsureMigrated(t *testing.T) {
	t.Helper()
	ApplyMigrations(t, AdminDSN(t))
}

// ApplyMigrations applies every pending migration to the database at dsn.
func ApplyMigrations(t *testing.T, dsn string) {
	t.Helper()
	src, err := iofs.New(db.MigrationsFS, "migrations")
	require.NoError(t, err)
	m, err := migrate.NewWithSourceInstance("iofs", src, dsn)
	require.NoError(t, err)
	defer func() {
		srcErr, dbErr := m.Close()
		require.NoError(t, srcErr)
		require.NoError(t, dbErr)
	}()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		require.NoError(t, err)
	}
}

// AdminPool returns a connection pool for the privileged role, with the schema
// migrated. The pool is closed when the test ends.
func AdminPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	EnsureMigrated(t)
	return openPool(t, AdminDSN(t))
}

// AppPool returns a connection pool for the restricted nvelope_app role, with
// the schema migrated. This is the role the application uses at runtime, so
// RLS policies apply to it. The pool is closed when the test ends.
func AppPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	EnsureMigrated(t)
	return openPool(t, AppDSN(t))
}

func openPool(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), dsn)
	require.NoError(t, err)
	require.NoError(t, pool.Ping(context.Background()))
	t.Cleanup(pool.Close)
	return pool
}

// ScratchDatabaseDSN creates a fresh, uniquely named database and returns an
// admin DSN pointing at it. The database is dropped when the test ends. Use it
// for destructive tests (such as the migration round-trip) that must not
// disturb the shared test database.
func ScratchDatabaseDSN(t *testing.T) string {
	t.Helper()
	admin := AdminDSN(t)
	u, err := url.Parse(admin)
	require.NoError(t, err)

	suffix := make([]byte, 8)
	_, err = rand.Read(suffix)
	require.NoError(t, err)
	name := "nvelope_test_" + hex.EncodeToString(suffix)
	quoted := pgx.Identifier{name}.Sanitize()

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, admin)
	require.NoError(t, err)
	_, err = conn.Exec(ctx, "CREATE DATABASE "+quoted)
	require.NoError(t, err)
	require.NoError(t, conn.Close(ctx))

	t.Cleanup(func() {
		c, err := pgx.Connect(ctx, admin)
		if err != nil {
			return
		}
		_, _ = c.Exec(ctx, "DROP DATABASE IF EXISTS "+quoted+" WITH (FORCE)")
		_ = c.Close(ctx)
	})

	u.Path = "/" + name
	return u.String()
}

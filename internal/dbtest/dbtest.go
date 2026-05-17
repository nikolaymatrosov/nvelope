// Package dbtest provides shared helpers for integration tests that need a
// real PostgreSQL database. By default the helpers start (or reuse) a Postgres
// container via testcontainers-go, so a Docker daemon must be available.
// Setting NVELOPE_MIGRATE_DATABASE_URL or NVELOPE_DATABASE_URL points the suite
// at an external database instead.
package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/db"
)

// RandString returns a short random lowercase-hex string, useful for unique
// test identifiers such as emails and tenant slugs.
func RandString() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// AdminDSN returns the privileged PostgreSQL DSN used to run migrations. It
// resolves to a testcontainers-managed Postgres container, or to the database
// named by NVELOPE_MIGRATE_DATABASE_URL / NVELOPE_DATABASE_URL when either is
// set. The test fails when no database can be obtained (for example when Docker
// is not running).
func AdminDSN(t *testing.T) string {
	t.Helper()
	dsn, err := adminDSN()
	require.NoError(t, err)
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

// EnsureMigrated applies every pending migration to the shared admin database.
// Migrations run only once per test binary; subsequent calls are no-ops, so it
// is safe to call from every integration test.
func EnsureMigrated(t *testing.T) {
	t.Helper()
	require.NoError(t, ensureMigratedOnce(AdminDSN(t)))
}

// ApplyMigrations applies every pending migration to the database at dsn. Use
// it for one-off databases such as those from ScratchDatabaseDSN; for the
// shared test database prefer EnsureMigrated.
func ApplyMigrations(t *testing.T, dsn string) {
	t.Helper()
	require.NoError(t, applyMigrations(dsn))
}

// applyMigrations applies every pending migration to the database at dsn.
func applyMigrations(dsn string) error {
	src, err := iofs.New(db.MigrationsFS, "migrations")
	if err != nil {
		return err
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, dsn)
	if err != nil {
		return err
	}
	defer m.Close()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
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

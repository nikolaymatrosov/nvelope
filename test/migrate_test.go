// Package test holds cross-cutting integration tests. The migration test
// runs against a real PostgreSQL instance (CI provides one); it is skipped
// when NVELOPE_DATABASE_URL is unset.
package test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/nvelope/nvelope/internal/db"
)

func TestBaselineMigrationRoundTrip(t *testing.T) {
	dsn := os.Getenv("NVELOPE_DATABASE_URL")
	if dsn == "" {
		t.Skip("NVELOPE_DATABASE_URL not set; skipping migration integration test")
	}

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

	// Ensure a clean starting point.
	m := newMigrator()
	if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		require.NoError(t, err)
	}
	closeMigrator(m)

	// Apply: the baseline must apply and record version 1.
	m = newMigrator()
	require.NoError(t, m.Up())
	v, dirty, err := m.Version()
	require.NoError(t, err)
	require.Equal(t, uint(1), v)
	require.False(t, dirty)
	closeMigrator(m)
	require.True(t, extensionExists(t, dsn, "pgcrypto"), "pgcrypto should exist after up")

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
	require.False(t, extensionExists(t, dsn, "pgcrypto"), "pgcrypto should be gone after down")

	// Re-apply cleanly so the database is left migrated.
	m = newMigrator()
	require.NoError(t, m.Up())
	closeMigrator(m)
}

func extensionExists(t *testing.T, dsn, name string) bool {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	require.NoError(t, err)
	defer func() { _ = conn.Close(ctx) }()

	var exists bool
	require.NoError(t, conn.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = $1)", name).Scan(&exists))
	return exists
}

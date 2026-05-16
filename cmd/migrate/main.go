// Command migrate applies and reverts nvelope's versioned database
// migrations. It wraps golang-migrate with the migrations embedded at build
// time and connects via NVELOPE_MIGRATE_DATABASE_URL (the privileged role),
// falling back to NVELOPE_DATABASE_URL when that is unset.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/nvelope/nvelope/internal/config"
	"github.com/nvelope/nvelope/internal/db"
)

const migrationsDir = "internal/db/migrations"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "up":
		exitOn(runUp())
	case "down":
		exitOn(runDown())
	case "version":
		exitOn(runVersion())
	case "create":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "migrate create: missing migration name")
			os.Exit(2)
		}
		exitOn(runCreate(os.Args[2]))
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: migrate <up|down|version|create NAME>")
}

func exitOn(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func newMigrator() (*migrate.Migrate, error) {
	cfg, err := config.Load(".env")
	if err != nil {
		return nil, err
	}
	src, err := iofs.New(db.MigrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("loading embedded migrations: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, cfg.MigrateDatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}
	return m, nil
}

func closeMigrator(m *migrate.Migrate) {
	if srcErr, dbErr := m.Close(); srcErr != nil || dbErr != nil {
		fmt.Fprintln(os.Stderr, "migrate: error closing:", errors.Join(srcErr, dbErr))
	}
}

func runUp() error {
	m, err := newMigrator()
	if err != nil {
		return err
	}
	defer closeMigrator(m)

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	fmt.Println("migrations up to date")
	return nil
}

func runDown() error {
	m, err := newMigrator()
	if err != nil {
		return err
	}
	defer closeMigrator(m)

	if err := m.Steps(-1); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	fmt.Println("reverted one migration")
	return nil
}

func runVersion() error {
	m, err := newMigrator()
	if err != nil {
		return err
	}
	defer closeMigrator(m)

	v, dirty, err := m.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		fmt.Println("schema version: 0 (no migrations applied)")
		return nil
	}
	if err != nil {
		return err
	}
	fmt.Printf("schema version: %d (dirty=%t)\n", v, dirty)
	return nil
}

func runCreate(name string) error {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("reading %s: %w", migrationsDir, err)
	}

	maxSeq := 0
	for _, e := range entries {
		base := e.Name()
		if i := strings.IndexByte(base, '_'); i > 0 {
			if n, err := strconv.Atoi(base[:i]); err == nil && n > maxSeq {
				maxSeq = n
			}
		}
	}

	seq := fmt.Sprintf("%06d", maxSeq+1)
	slug := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(name)), " ", "_")
	for _, dir := range []string{"up", "down"} {
		path := filepath.Join(migrationsDir, fmt.Sprintf("%s_%s.%s.sql", seq, slug, dir))
		content := fmt.Sprintf("-- %s (%s migration)\n", slug, dir)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
		fmt.Println("created", path)
	}
	return nil
}

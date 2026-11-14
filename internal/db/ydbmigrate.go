package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"

	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"
)

// YDBMigrationsFS holds the YDB schema migrations — one file per ported
// bounded context — applied by MigrateYDB for the serverless runtime.
//
//go:embed ydbmigrations/*.up.sql
var YDBMigrationsFS embed.FS

// MigrateYDB applies every embedded YDB migration that schema_migrations does
// not record yet, in version order.
func MigrateYDB(ctx context.Context, d *ydb.Driver) error {
	sub, err := fs.Sub(YDBMigrationsFS, "ydbmigrations")
	if err != nil {
		return fmt.Errorf("opening embedded ydb migrations: %w", err)
	}
	return MigrateYDBFrom(ctx, d.Query(), sub)
}

// MigrateYDBFrom applies the *.up.sql files at the root of fsys that
// schema_migrations does not record yet, in version order.
//
// YDB runs DDL outside transactions and caps query text at 10 KB, so every
// statement runs in its own Exec and a failure can leave a file half-applied.
// Each statement must therefore be idempotent (CREATE ... IF NOT EXISTS, DROP
// ... IF EXISTS): a re-run re-applies the unrecorded file from its first
// statement and converges. A version is recorded only after all of its
// statements succeed.
func MigrateYDBFrom(ctx context.Context, q query.Executor, fsys fs.FS) error {
	names, err := fs.Glob(fsys, "*.up.sql")
	if err != nil {
		return fmt.Errorf("listing ydb migrations: %w", err)
	}
	type migration struct {
		version uint64
		name    string
	}
	migrations := make([]migration, 0, len(names))
	for _, name := range names {
		version, err := ydbMigrationVersion(name)
		if err != nil {
			return err
		}
		migrations = append(migrations, migration{version: version, name: name})
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].version < migrations[j].version })
	for i := 1; i < len(migrations); i++ {
		if migrations[i].version == migrations[i-1].version {
			return fmt.Errorf("ydb migrations %s and %s share version %d",
				migrations[i-1].name, migrations[i].name, migrations[i].version)
		}
	}

	if err := q.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
    version_id Uint64    NOT NULL,
    applied_at Timestamp NOT NULL,
    PRIMARY KEY (version_id)
);`); err != nil {
		return fmt.Errorf("creating schema_migrations: %w", err)
	}
	applied, err := appliedYDBVersions(ctx, q)
	if err != nil {
		return err
	}

	for _, m := range migrations {
		if applied[m.version] {
			continue
		}
		body, err := fs.ReadFile(fsys, m.name)
		if err != nil {
			return fmt.Errorf("reading ydb migration %s: %w", m.name, err)
		}
		for i, stmt := range splitYQLStatements(string(body)) {
			if err := q.Exec(ctx, stmt); err != nil {
				return fmt.Errorf("ydb migration %s, statement %d: %w", m.name, i+1, err)
			}
		}
		if err := q.Exec(ctx, `DECLARE $version_id AS Uint64;
UPSERT INTO schema_migrations (version_id, applied_at) VALUES ($version_id, CurrentUtcTimestamp());`,
			query.WithParameters(ydb.ParamsBuilder().Param("$version_id").Uint64(m.version).Build()),
		); err != nil {
			return fmt.Errorf("recording ydb migration %s: %w", m.name, err)
		}
	}
	return nil
}

// appliedYDBVersions reads the versions schema_migrations records.
func appliedYDBVersions(ctx context.Context, q query.Executor) (map[uint64]bool, error) {
	rs, err := q.QueryResultSet(ctx, "SELECT version_id FROM schema_migrations;")
	if err != nil {
		return nil, fmt.Errorf("reading schema_migrations: %w", err)
	}
	defer func() { _ = rs.Close(ctx) }()
	applied := map[uint64]bool{}
	for row, err := range rs.Rows(ctx) {
		if err != nil {
			return nil, fmt.Errorf("reading schema_migrations: %w", err)
		}
		var version uint64
		if err := row.Scan(&version); err != nil {
			return nil, fmt.Errorf("scanning schema_migrations: %w", err)
		}
		applied[version] = true
	}
	return applied, nil
}

// ydbMigrationVersion parses the leading number of a migration file name
// ("000001_sending.up.sql" → 1).
func ydbMigrationVersion(name string) (uint64, error) {
	digits, _, ok := strings.Cut(path.Base(name), "_")
	if !ok {
		return 0, fmt.Errorf("ydb migration %s: name must be <version>_<name>.up.sql", name)
	}
	version, err := strconv.ParseUint(digits, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("ydb migration %s: bad version: %w", name, err)
	}
	return version, nil
}

// splitYQLStatements splits a migration file into statements: a statement
// ends on a line whose trimmed text ends in ";". Chunks holding only blank
// lines and "--" comments are dropped.
//
// ponytail: line-based, so a ";" at the end of a line inside a string literal
// would split a statement, and so would a "--" comment line ending in ";"
// inside one; keep migration literals and comments off line ends. Use a real
// YQL lexer if that ever bites.
func splitYQLStatements(sql string) []string {
	var out []string
	var cur strings.Builder
	flush := func() {
		if s := strings.TrimSpace(cur.String()); hasYQLCode(s) {
			out = append(out, s)
		}
		cur.Reset()
	}
	for _, line := range strings.Split(sql, "\n") {
		cur.WriteString(line)
		cur.WriteByte('\n')
		if strings.HasSuffix(strings.TrimSpace(line), ";") {
			flush()
		}
	}
	flush()
	return out
}

// hasYQLCode reports whether s holds anything besides blank lines and "--"
// comments.
func hasYQLCode(s string) bool {
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" && !strings.HasPrefix(t, "--") {
			return true
		}
	}
	return false
}

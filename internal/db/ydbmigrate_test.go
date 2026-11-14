package db_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/db"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

func TestMigrateYDBAppliesEmbeddedSchema(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d := dbtest.YDBMigrated(t)

	// sending_domains comes from 000001_sending.up.sql.
	row, err := d.Query().QueryRow(ctx, "SELECT COUNT(*) AS n FROM sending_domains;")
	require.NoError(t, err)
	var n uint64
	require.NoError(t, row.Scan(&n))

	require.NoError(t, db.MigrateYDB(ctx, d), "re-running over an up-to-date schema is a no-op")
}

func TestMigrateYDBConvergesAfterPartialFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := dbtest.YDBMigrated(t).Query()

	// A version far above the real migrations and unique per run: the test
	// shares schema_migrations with the embedded set.
	version := 900_000_000 + rand.IntN(99_999_999)
	file := fmt.Sprintf("%d_partial.up.sql", version)
	tableA := fmt.Sprintf("mig_a_%d", version)
	tableB := fmt.Sprintf("mig_b_%d", version)
	createA := "CREATE TABLE IF NOT EXISTS " + tableA + " (id Utf8 NOT NULL, PRIMARY KEY (id));"
	createB := "CREATE TABLE IF NOT EXISTS " + tableB + " (id Utf8 NOT NULL, PRIMARY KEY (id));"

	broken := fstest.MapFS{file: {Data: []byte(createA + "\nTHIS IS NOT YQL;\n")}}
	require.Error(t, db.MigrateYDBFrom(ctx, q, broken), "the second statement fails")
	require.False(t, ydbVersionRecorded(t, q, version), "a half-applied file is not recorded")

	fixed := fstest.MapFS{file: {Data: []byte(createA + "\n" + createB + "\n")}}
	require.NoError(t, db.MigrateYDBFrom(ctx, q, fixed),
		"the re-run re-applies the file from its first statement and converges")
	require.True(t, ydbVersionRecorded(t, q, version))
	for _, table := range []string{tableA, tableB} {
		_, err := q.QueryRow(ctx, "SELECT COUNT(*) AS n FROM "+table+";")
		require.NoError(t, err, table)
	}
}

func ydbVersionRecorded(t *testing.T, q query.Executor, version int) bool {
	t.Helper()
	row, err := q.QueryRow(context.Background(), `DECLARE $version_id AS Uint64;
SELECT COUNT(*) AS n FROM schema_migrations WHERE version_id = $version_id;`,
		query.WithParameters(ydb.ParamsBuilder().Param("$version_id").Uint64(uint64(version)).Build()))
	require.NoError(t, err)
	var n uint64
	require.NoError(t, row.Scan(&n))
	return n == 1
}

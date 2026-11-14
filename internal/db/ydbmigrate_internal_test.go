package db

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

func TestSplitYQLStatements(t *testing.T) {
	t.Parallel()
	got := splitYQLStatements(`-- header comment

CREATE TABLE IF NOT EXISTS a (
    id Utf8 NOT NULL,
    PRIMARY KEY (id)
);
-- between
CREATE TABLE IF NOT EXISTS b (id Utf8 NOT NULL, PRIMARY KEY (id));

-- trailing comment only
`)
	require.Equal(t, []string{
		"-- header comment\n\nCREATE TABLE IF NOT EXISTS a (\n    id Utf8 NOT NULL,\n    PRIMARY KEY (id)\n);",
		"-- between\nCREATE TABLE IF NOT EXISTS b (id Utf8 NOT NULL, PRIMARY KEY (id));",
	}, got)
}

func TestYDBMigrationVersion(t *testing.T) {
	t.Parallel()
	v, err := ydbMigrationVersion("000012_audience.up.sql")
	require.NoError(t, err)
	require.Equal(t, uint64(12), v)

	_, err = ydbMigrationVersion("audience.up.sql")
	require.Error(t, err, "a name without a <version>_ prefix is rejected")

	_, err = ydbMigrationVersion("v1_audience.up.sql")
	require.Error(t, err, "a non-numeric version is rejected")
}

func TestMigrateYDBFromRejectsDuplicateVersions(t *testing.T) {
	t.Parallel()
	// A nil executor is safe: validation runs before any statement executes.
	err := MigrateYDBFrom(context.Background(), nil, fstest.MapFS{
		"000001_a.up.sql": {Data: []byte("SELECT 1;")},
		"000001_b.up.sql": {Data: []byte("SELECT 2;")},
	})
	require.ErrorContains(t, err, "000001_a.up.sql")
	require.ErrorContains(t, err, "000001_b.up.sql")
}

package dbtest_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

func TestYDBRoundTrip(t *testing.T) {
	t.Parallel()
	db := dbtest.YDB(t)

	row, err := db.Query().QueryRow(context.Background(), "SELECT 42 AS answer")
	require.NoError(t, err)

	var answer int32
	require.NoError(t, row.Scan(&answer))
	require.Equal(t, int32(42), answer)
}

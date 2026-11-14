package jobs_test

import (
	"context"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
)

// Separate pools stand in for the separate test binaries (or replicas) that
// migrate one fresh database at once under `go test ./...`.
func TestMigrateIsSafeUnderConcurrency(t *testing.T) {
	ctx := context.Background()
	dsn := dbtest.ScratchDatabaseDSN(t)

	errs := make([]error, 4)
	var wg sync.WaitGroup
	for i := range errs {
		pool, err := pgxpool.New(ctx, dsn)
		require.NoError(t, err)
		t.Cleanup(pool.Close)
		wg.Go(func() { errs[i] = jobs.Migrate(ctx, pool) })
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
}

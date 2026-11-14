package ydbthrottle

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

// newTestCounter returns a Counter on the test YDB whose clock reads *clock,
// so window arithmetic is deterministic.
func newTestCounter(t *testing.T, clock *time.Time) *Counter {
	t.Helper()
	c := New(dbtest.YDBMigrated(t))
	c.now = func() time.Time { return *clock }
	return c
}

// testKey returns a key unique to this test run; the table is shared.
func testKey(name string) string { return "test:" + uuid.NewString() + ":" + name }

func TestAllowAdmitsUpToMaxThenDenies(t *testing.T) {
	t.Parallel()
	clock := time.Date(2026, 11, 14, 12, 0, 30, 0, time.UTC)
	c := newTestCounter(t, &clock)
	w := Window{Key: testKey("max"), Max: 3, Size: time.Minute}
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		ok, _, err := c.Allow(ctx, w)
		require.NoError(t, err)
		require.True(t, ok, "hit %d is within the limit", i+1)
	}
	ok, retryAfter, err := c.Allow(ctx, w)
	require.NoError(t, err)
	require.False(t, ok, "the fourth hit exceeds Max")
	require.Equal(t, 30*time.Second, retryAfter, "the window [12:00, 12:01) ends 30s after 12:00:30")
}

func TestAllowRollsOverToTheNextWindow(t *testing.T) {
	t.Parallel()
	clock := time.Date(2026, 11, 14, 12, 0, 59, 0, time.UTC)
	c := newTestCounter(t, &clock)
	w := Window{Key: testKey("rollover"), Max: 1, Size: time.Minute}
	ctx := context.Background()

	ok, _, err := c.Allow(ctx, w)
	require.NoError(t, err)
	require.True(t, ok)
	ok, _, err = c.Allow(ctx, w)
	require.NoError(t, err)
	require.False(t, ok)

	clock = time.Date(2026, 11, 14, 12, 1, 0, 0, time.UTC)
	ok, _, err = c.Allow(ctx, w)
	require.NoError(t, err)
	require.True(t, ok, "a new window starts with a fresh budget")
}

func TestAllowCountsOnlyWhenEveryWindowHasRoom(t *testing.T) {
	t.Parallel()
	clock := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	c := newTestCounter(t, &clock)
	tight := Window{Key: testKey("tight"), Max: 1, Size: time.Minute}
	loose := Window{Key: testKey("loose"), Max: 5, Size: time.Minute}
	ctx := context.Background()

	ok, _, err := c.Allow(ctx, tight, loose)
	require.NoError(t, err)
	require.True(t, ok)
	ok, retryAfter, err := c.Allow(ctx, tight, loose)
	require.NoError(t, err)
	require.False(t, ok, "the tight window is full")
	require.Equal(t, time.Minute, retryAfter)

	// The denied hit did not count against the loose window: it still has
	// exactly 4 of its 5 hits left.
	for i := 0; i < 4; i++ {
		ok, _, err := c.Allow(ctx, loose)
		require.NoError(t, err)
		require.True(t, ok, "loose hit %d", i+2)
	}
	ok, _, err = c.Allow(ctx, loose)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestAllowRejectsInvalidWindows(t *testing.T) {
	t.Parallel()
	clock := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	c := newTestCounter(t, &clock)
	ctx := context.Background()

	_, _, err := c.Allow(ctx)
	require.Error(t, err, "at least one window is required")
	_, _, err = c.Allow(ctx, Window{Key: testKey("zero"), Max: 0, Size: time.Minute})
	require.Error(t, err)
	_, _, err = c.Allow(ctx, Window{Key: testKey("nosize"), Max: 1})
	require.Error(t, err)
	_, _, err = c.Allow(ctx, Window{Max: 1, Size: time.Minute})
	require.Error(t, err)
}

func TestKeyedSeparatesKeys(t *testing.T) {
	t.Parallel()
	clock := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	k := newTestCounter(t, &clock).Keyed(testKey("resend")+":", 1, time.Hour)
	ctx := context.Background()

	ok, err := k.Allow(ctx, "user-a")
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = k.Allow(ctx, "user-a")
	require.NoError(t, err)
	require.False(t, ok, "user-a used its one hit this hour")
	ok, err = k.Allow(ctx, "user-b")
	require.NoError(t, err)
	require.True(t, ok, "another key has its own window")
}

// Keyed matches the single-key throttle ports (auth ResendThrottle, audience
// SubmissionThrottle) structurally; Plan C's wiring relies on it.
var _ interface {
	Allow(context.Context, string) (bool, error)
} = (*Keyed)(nil)

// Concurrent first hits all see "no row" for a fresh window; serializable
// isolation plus the SDK's DoTx retry must still admit exactly Max.
func TestAllowIsConsistentUnderConcurrency(t *testing.T) {
	t.Parallel()
	clock := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	c := newTestCounter(t, &clock)
	w := Window{Key: testKey("race"), Max: 5, Size: time.Minute}

	const workers = 20
	var admitted, failed atomic.Int32
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			ok, _, err := c.Allow(context.Background(), w)
			switch {
			case err != nil:
				failed.Add(1)
				t.Errorf("Allow: %v", err)
			case ok:
				admitted.Add(1)
			}
		})
	}
	wg.Wait()

	require.Zero(t, failed.Load())
	require.EqualValues(t, w.Max, admitted.Load())
}

// Package ydbthrottle is a fixed-window rate counter on YDB for the serverless
// runtime, where Redis is unavailable. It backs the campaign send limiter and
// the single-key anti-abuse throttles.
package ydbthrottle

import (
	"context"
	"errors"
	"fmt"
	"time"

	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"
)

// Window is one fixed-window allowance: at most Max hits on Key per Size.
type Window struct {
	Key  string
	Max  int
	Size time.Duration
}

// Counter counts hits in fixed windows on the throttle_windows table
// (internal/db/ydbmigrations/000002_throttle.up.sql). It is safe for
// concurrent use.
//
// ponytail: fixed windows admit up to 2x Max across a window boundary, and
// every hit on a shared key (the global send window) writes the same row, so
// heavy concurrency turns into transaction retries. Move to a sliding log or
// sharded counters if either matters.
type Counter struct {
	db  *ydb.Driver
	now func() time.Time
}

// New builds a Counter over the driver.
func New(d *ydb.Driver) *Counter {
	return &Counter{db: d, now: time.Now}
}

const selectHits = `DECLARE $throttle_key AS Utf8;
DECLARE $window_start AS Timestamp;
SELECT hits FROM throttle_windows
WHERE throttle_key = $throttle_key AND window_start = $window_start;`

const upsertHits = `DECLARE $throttle_key AS Utf8;
DECLARE $window_start AS Timestamp;
DECLARE $hits AS Uint64;
DECLARE $expire_at AS Timestamp;
UPSERT INTO throttle_windows (throttle_key, window_start, hits, expire_at)
VALUES ($throttle_key, $window_start, $hits, $expire_at);`

// Allow admits one hit only when every window has room, and only then counts
// it in all of them, so a denial never spends one window's budget while
// another is full. When denied, retryAfter is the longest wait until a full
// window rolls over. It runs as one serializable transaction, which the SDK
// retries on conflict.
func (c *Counter) Allow(ctx context.Context, windows ...Window) (allowed bool, retryAfter time.Duration, err error) {
	if len(windows) == 0 {
		return false, 0, errors.New("ydbthrottle: at least one window is required")
	}
	for _, w := range windows {
		if w.Key == "" || w.Max <= 0 || w.Size <= 0 {
			return false, 0, fmt.Errorf("ydbthrottle: invalid window %+v: key, max and size are required", w)
		}
	}
	now := c.now().UTC()
	err = c.db.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		// The closure may run more than once (a retried transaction), so the
		// outcome is recomputed from scratch on every run.
		allowed, retryAfter = true, 0
		hits := make([]uint64, len(windows))
		for i, w := range windows {
			start := now.Truncate(w.Size)
			n, err := readHits(ctx, tx, w.Key, start)
			if err != nil {
				return err
			}
			hits[i] = n
			if n >= uint64(w.Max) {
				allowed = false
				if wait := start.Add(w.Size).Sub(now); wait > retryAfter {
					retryAfter = wait
				}
			}
		}
		if !allowed {
			return nil
		}
		for i, w := range windows {
			start := now.Truncate(w.Size)
			if err := tx.Exec(ctx, upsertHits, query.WithParameters(ydb.ParamsBuilder().
				Param("$throttle_key").Text(w.Key).
				Param("$window_start").Timestamp(start).
				Param("$hits").Uint64(hits[i]+1).
				Param("$expire_at").Timestamp(start.Add(w.Size)).
				Build())); err != nil {
				return fmt.Errorf("ydbthrottle: counting a hit on %q: %w", w.Key, err)
			}
		}
		return nil
	})
	if err != nil {
		return false, 0, err
	}
	return allowed, retryAfter, nil
}

// readHits returns the hits recorded for key in the window starting at start,
// or 0 when the window has no row yet.
func readHits(ctx context.Context, tx query.TxActor, key string, start time.Time) (uint64, error) {
	row, err := tx.QueryRow(ctx, selectHits, query.WithParameters(ydb.ParamsBuilder().
		Param("$throttle_key").Text(key).
		Param("$window_start").Timestamp(start).
		Build()))
	if errors.Is(err, query.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("ydbthrottle: reading %q: %w", key, err)
	}
	var n uint64
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("ydbthrottle: scanning %q: %w", key, err)
	}
	return n, nil
}

// Keyed is a single-window throttle per key, namespaced by a prefix. Its
// Allow method matches the auth ResendThrottle and audience SubmissionThrottle
// ports.
type Keyed struct {
	counter *Counter
	prefix  string
	max     int
	size    time.Duration
}

// Keyed builds a per-key throttle admitting max hits per size window.
func (c *Counter) Keyed(prefix string, max int, size time.Duration) *Keyed {
	return &Keyed{counter: c, prefix: prefix, max: max, size: size}
}

// Allow reports whether one more hit on key fits its window, counting it if so.
func (k *Keyed) Allow(ctx context.Context, key string) (bool, error) {
	ok, _, err := k.counter.Allow(ctx, Window{Key: k.prefix + key, Max: k.max, Size: k.size})
	return ok, err
}

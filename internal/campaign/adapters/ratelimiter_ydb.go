package adapters

import (
	"context"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/ydbthrottle"
)

// YDBRateLimiter adapts the YDB fixed-window counter to the campaign context's
// RateLimiter port, for the serverless runtime: one send must fit both the
// tenant's window and the platform-wide window.
type YDBRateLimiter struct {
	counter *ydbthrottle.Counter
	global  domain.Limit
}

var _ domain.RateLimiter = (*YDBRateLimiter)(nil)

// NewYDBRateLimiter builds a YDBRateLimiter with the platform-wide cap.
func NewYDBRateLimiter(counter *ydbthrottle.Counter, global domain.Limit) *YDBRateLimiter {
	if counter == nil {
		panic("nil throttle counter")
	}
	return &YDBRateLimiter{counter: counter, global: global}
}

// Allow checks the per-tenant and global windows together for one send.
func (r *YDBRateLimiter) Allow(ctx context.Context, tenantID string,
	perTenant domain.Limit) (bool, time.Duration, error) {
	return r.counter.Allow(ctx,
		ydbthrottle.Window{Key: "rl:tenant:" + tenantID, Max: perTenant.Max, Size: perTenant.Window},
		ydbthrottle.Window{Key: "rl:global", Max: r.global.Max, Size: r.global.Window})
}

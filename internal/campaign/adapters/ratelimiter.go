package adapters

import (
	"context"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/ratelimit"
)

// RateLimiter adapts the shared Redis sliding-window limiter to the campaign
// context's domain-owned RateLimiter interface.
type RateLimiter struct {
	limiter *ratelimit.Limiter
}

var _ domain.RateLimiter = (*RateLimiter)(nil)

// NewRateLimiter builds a RateLimiter over the shared Redis limiter.
func NewRateLimiter(limiter *ratelimit.Limiter) *RateLimiter {
	if limiter == nil {
		panic("nil rate limiter")
	}
	return &RateLimiter{limiter: limiter}
}

// Allow checks the per-tenant and global windows for one send.
func (r *RateLimiter) Allow(ctx context.Context, tenantID string,
	perTenant domain.Limit) (bool, time.Duration, error) {
	return r.limiter.Allow(ctx, tenantID, ratelimit.Limit{
		Max:    perTenant.Max,
		Window: perTenant.Window,
	})
}

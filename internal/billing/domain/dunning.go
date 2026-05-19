package domain

import "time"

// DunningPolicy is the bounded-retry schedule applied to a failed charge: how
// many attempts are made before the subscription is suspended, and how far
// apart the retries are spaced.
type DunningPolicy struct {
	maxAttempts   int
	retryInterval time.Duration
}

// NewDunningPolicy builds a dunning policy. A non-positive maxAttempts falls
// back to one attempt; a non-positive retryInterval falls back to 72 hours.
func NewDunningPolicy(maxAttempts int, retryInterval time.Duration) DunningPolicy {
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	if retryInterval <= 0 {
		retryInterval = 72 * time.Hour
	}
	return DunningPolicy{maxAttempts: maxAttempts, retryInterval: retryInterval}
}

// MaxAttempts returns the number of failed charges tolerated before suspension.
func (p DunningPolicy) MaxAttempts() int { return p.maxAttempts }

// RetryInterval returns the spacing between dunning retries.
func (p DunningPolicy) RetryInterval() time.Duration { return p.retryInterval }

// NextAttemptAt returns when the next dunning retry is due, given the time of
// the failure.
func (p DunningPolicy) NextAttemptAt(failedAt time.Time) time.Time {
	return failedAt.Add(p.retryInterval).UTC()
}

// IsExhausted reports whether a charge that has failed attemptCount times has
// run out of retries — the subscription must be suspended.
func (p DunningPolicy) IsExhausted(attemptCount int) bool {
	return attemptCount >= p.maxAttempts
}

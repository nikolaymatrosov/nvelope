package adapters

import (
	"context"
	"fmt"
	"time"

	"github.com/riverqueue/river"

	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
)

// VerificationCleanupWorker is the River worker that prunes expired
// email-verification tokens, so the email_verification_tokens table stays
// bounded to the tokens still within their validity window.
type VerificationCleanupWorker struct {
	river.WorkerDefaults[jobs.VerificationCleanupArgs]
	verifications *EmailVerifications
}

// NewVerificationCleanupWorker builds the cleanup worker, failing fast on a nil
// dependency.
func NewVerificationCleanupWorker(verifications *EmailVerifications) *VerificationCleanupWorker {
	if verifications == nil {
		panic("nil verifications repository")
	}
	return &VerificationCleanupWorker{verifications: verifications}
}

// Work deletes every verification token that has passed its expiry.
func (w *VerificationCleanupWorker) Work(ctx context.Context,
	_ *river.Job[jobs.VerificationCleanupArgs]) error {

	if _, err := w.verifications.DeleteExpiredBefore(ctx, time.Now()); err != nil {
		return fmt.Errorf("sweeping expired verification tokens: %w", err)
	}
	return nil
}

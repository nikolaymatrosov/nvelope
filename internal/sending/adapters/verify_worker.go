package adapters

import (
	"context"
	"errors"
	"time"

	"github.com/riverqueue/river"

	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
	"github.com/nikolaymatrosov/nvelope/internal/sending/domain"
)

// VerifyWorker is the River worker that polls a sending domain's verification
// status. It re-checks the provider, transitions the domain to verified or
// failed, and snoozes while the domain is still pending and within its
// verification window.
type VerifyWorker struct {
	river.WorkerDefaults[jobs.DomainVerifyArgs]
	domains  domain.SendingDomainRepository
	verifier domain.IdentityVerifier
	interval time.Duration
	window   time.Duration
}

// NewVerifyWorker builds the verify worker, failing fast on a nil dependency.
func NewVerifyWorker(domains domain.SendingDomainRepository, verifier domain.IdentityVerifier,
	interval, window time.Duration) *VerifyWorker {
	if domains == nil || verifier == nil {
		panic("nil dependency")
	}
	return &VerifyWorker{domains: domains, verifier: verifier, interval: interval, window: window}
}

// Work runs one verification poll.
func (w *VerifyWorker) Work(ctx context.Context, job *river.Job[jobs.DomainVerifyArgs]) error {
	tenantID, domainID := job.Args.TenantID, job.Args.DomainID

	d, err := w.domains.Get(ctx, tenantID, domainID)
	if err != nil {
		if errors.Is(err, domain.ErrDomainNotFound) {
			return nil // the domain was deleted — nothing to poll
		}
		return err
	}
	if !d.IsPending() {
		return nil // verified or failed — terminal
	}

	verified, checkErr := w.verifier.Check(ctx, d.IdentityRef())
	now := time.Now()

	var snooze bool
	err = w.domains.Update(ctx, tenantID, domainID,
		func(d *domain.SendingDomain) (*domain.SendingDomain, error) {
			if !d.IsPending() {
				return d, nil
			}
			d.RecordCheck(now)
			if checkErr == nil && verified {
				return d, d.MarkVerified(now)
			}
			if now.Sub(d.CreatedAt()) > w.window {
				reason := "the domain did not verify within the allowed window"
				if checkErr != nil {
					reason = "verification check failed before the window elapsed"
				}
				return d, d.MarkFailed(reason)
			}
			snooze = true
			return d, nil
		})
	if err != nil {
		return err
	}
	if snooze {
		return river.JobSnooze(w.interval)
	}
	return nil
}

package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
	"github.com/nikolaymatrosov/nvelope/internal/sending/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/sending/domain"
)

// fakeVerifier is a deterministic IdentityVerifier for component tests.
type fakeVerifier struct {
	verified bool
	err      error
}

func (f fakeVerifier) Check(context.Context, string) (bool, error) {
	return f.verified, f.err
}

// backdateDomain rewrites a domain's created_at via the admin pool (which
// bypasses RLS) so the verification window can be exercised in tests.
func backdateDomain(t *testing.T, id string, age time.Duration) {
	t.Helper()
	admin := dbtest.AdminPool(t)
	_, err := admin.Exec(context.Background(),
		"UPDATE sending_domains SET created_at = $1 WHERE id = $2",
		time.Now().Add(-age), id)
	require.NoError(t, err)
}

func verifyJob(tenantID, domainID string) *river.Job[jobs.DomainVerifyArgs] {
	return &river.Job[jobs.DomainVerifyArgs]{
		Args: jobs.DomainVerifyArgs{TenantID: tenantID, DomainID: domainID},
	}
}

func TestVerifyWorkerVerifiedPath(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewSendingDomains(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	id, err := repo.Add(ctx, tenantID, newDomain(t, tenantID, "mail.acme.com"))
	require.NoError(t, err)

	w := adapters.NewVerifyWorker(repo, fakeVerifier{verified: true}, time.Minute, 72*time.Hour)
	require.NoError(t, w.Work(ctx, verifyJob(tenantID, id)))

	got, err := repo.Get(ctx, tenantID, id)
	require.NoError(t, err)
	require.Equal(t, domain.StatusVerified, got.Status())
	require.NotNil(t, got.LastCheckedAt())
}

func TestVerifyWorkerFailedAfterWindow(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewSendingDomains(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	id, err := repo.Add(ctx, tenantID, newDomain(t, tenantID, "mail.acme.com"))
	require.NoError(t, err)
	backdateDomain(t, id, 100*time.Hour)

	w := adapters.NewVerifyWorker(repo, fakeVerifier{verified: false}, time.Minute, 72*time.Hour)
	require.NoError(t, w.Work(ctx, verifyJob(tenantID, id)))

	got, err := repo.Get(ctx, tenantID, id)
	require.NoError(t, err)
	require.Equal(t, domain.StatusFailed, got.Status())
	require.NotEmpty(t, got.FailureReason())
}

func TestVerifyWorkerSnoozesWhilePending(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewSendingDomains(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	id, err := repo.Add(ctx, tenantID, newDomain(t, tenantID, "mail.acme.com"))
	require.NoError(t, err)

	w := adapters.NewVerifyWorker(repo, fakeVerifier{verified: false}, time.Minute, 72*time.Hour)
	err = w.Work(ctx, verifyJob(tenantID, id))
	require.Error(t, err, "an unverified domain within the window snoozes")

	got, err := repo.Get(ctx, tenantID, id)
	require.NoError(t, err)
	require.Equal(t, domain.StatusPending, got.Status())
	require.NotNil(t, got.LastCheckedAt(), "the poll is still recorded")
}

func TestVerifyWorkerTerminalDomainNoOp(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewSendingDomains(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	id, err := repo.Add(ctx, tenantID, newDomain(t, tenantID, "mail.acme.com"))
	require.NoError(t, err)
	require.NoError(t, repo.Update(ctx, tenantID, id,
		func(d *domain.SendingDomain) (*domain.SendingDomain, error) {
			return d, d.MarkVerified(time.Now())
		}))

	w := adapters.NewVerifyWorker(repo, fakeVerifier{verified: false}, time.Minute, 72*time.Hour)
	require.NoError(t, w.Work(ctx, verifyJob(tenantID, id)), "a verified domain is left untouched")
}

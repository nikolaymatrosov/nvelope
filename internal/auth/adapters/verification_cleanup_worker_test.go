package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/auth/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
)

func TestVerificationCleanupWorkerSweepsExpiredTokens(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	users := adapters.NewUsers(pool)
	repo := adapters.NewEmailVerifications(pool)
	now := time.Now()

	user, _ := newUser(t, "Ada")
	created, err := users.Create(ctx, user, "hash")
	require.NoError(t, err)
	expired := domain.HydrateEmailVerification("", created.ID(),
		now.Add(-time.Hour), now.Add(-2*time.Hour), nil)
	_, err = repo.Issue(ctx, expired, "cleanup-worker-expired-hash")
	require.NoError(t, err)

	worker := adapters.NewVerificationCleanupWorker(repo)
	require.NoError(t, worker.Work(ctx, &river.Job[jobs.VerificationCleanupArgs]{
		Args: jobs.VerificationCleanupArgs{},
	}))

	_, err = repo.GetByTokenHash(ctx, "cleanup-worker-expired-hash")
	require.ErrorIs(t, err, domain.ErrVerificationLinkInvalid,
		"the worker swept the expired token")
}

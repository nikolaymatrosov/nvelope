package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/auth/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

func TestEmailVerificationsIssueGetConsume(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	users := adapters.NewUsers(pool)
	repo := adapters.NewEmailVerifications(pool)

	u, _ := newUser(t, "Ada")
	created, err := users.Create(ctx, u, "hash")
	require.NoError(t, err)

	v, err := domain.NewEmailVerification(created.ID(), time.Hour)
	require.NoError(t, err)
	issued, err := repo.Issue(ctx, v, "token-hash-1")
	require.NoError(t, err)
	require.NotEmpty(t, issued.ID(), "the database assigns an id")

	got, err := repo.GetByTokenHash(ctx, "token-hash-1")
	require.NoError(t, err)
	require.Equal(t, created.ID(), got.UserID())
	require.False(t, got.IsConsumed())
	require.False(t, got.IsExpired(time.Now()))

	require.NoError(t, repo.Consume(ctx, got.ID(), time.Now()))
	consumed, err := repo.GetByTokenHash(ctx, "token-hash-1")
	require.NoError(t, err)
	require.True(t, consumed.IsConsumed(), "the row reflects the consumption")
}

func TestEmailVerificationsIssueSupersedesPending(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	users := adapters.NewUsers(pool)
	repo := adapters.NewEmailVerifications(pool)

	u, _ := newUser(t, "Ada")
	created, err := users.Create(ctx, u, "hash")
	require.NoError(t, err)

	first, err := domain.NewEmailVerification(created.ID(), time.Hour)
	require.NoError(t, err)
	_, err = repo.Issue(ctx, first, "first-hash")
	require.NoError(t, err)

	second, err := domain.NewEmailVerification(created.ID(), time.Hour)
	require.NoError(t, err)
	_, err = repo.Issue(ctx, second, "second-hash")
	require.NoError(t, err)

	// Issuing a fresh challenge deletes the prior unconsumed one (FR-012).
	_, err = repo.GetByTokenHash(ctx, "first-hash")
	require.ErrorIs(t, err, domain.ErrVerificationLinkInvalid)
	_, err = repo.GetByTokenHash(ctx, "second-hash")
	require.NoError(t, err)
}

func TestEmailVerificationsGetUnknownToken(t *testing.T) {
	t.Parallel()
	repo := adapters.NewEmailVerifications(dbtest.AppPool(t))

	_, err := repo.GetByTokenHash(context.Background(), "no-such-token-hash")
	require.ErrorIs(t, err, domain.ErrVerificationLinkInvalid)
}

func TestEmailVerificationsDeleteExpiredBefore(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	users := adapters.NewUsers(pool)
	repo := adapters.NewEmailVerifications(pool)
	now := time.Now()

	// A token still within its validity window survives the sweep.
	liveUser, _ := newUser(t, "Ada")
	createdLive, err := users.Create(ctx, liveUser, "hash")
	require.NoError(t, err)
	live, err := domain.NewEmailVerification(createdLive.ID(), time.Hour)
	require.NoError(t, err)
	_, err = repo.Issue(ctx, live, "delete-test-live-hash")
	require.NoError(t, err)

	// A long-expired token is swept.
	expiredUser, _ := newUser(t, "Grace")
	createdExpired, err := users.Create(ctx, expiredUser, "hash")
	require.NoError(t, err)
	expired := domain.HydrateEmailVerification("", createdExpired.ID(),
		now.Add(-time.Hour), now.Add(-2*time.Hour), nil)
	_, err = repo.Issue(ctx, expired, "delete-test-expired-hash")
	require.NoError(t, err)

	// A cutoff in the recent past sweeps only the long-expired token; other
	// tests' tokens all expire in the future, so they are untouched.
	_, err = repo.DeleteExpiredBefore(ctx, now.Add(-30*time.Minute))
	require.NoError(t, err)

	_, err = repo.GetByTokenHash(ctx, "delete-test-live-hash")
	require.NoError(t, err, "the live token survives the sweep")
	_, err = repo.GetByTokenHash(ctx, "delete-test-expired-hash")
	require.ErrorIs(t, err, domain.ErrVerificationLinkInvalid, "the expired token is gone")
}

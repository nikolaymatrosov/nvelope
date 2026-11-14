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
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.verifications(t)
		ctx := context.Background()
		userID := b.newUserID(t)
		tokenHash := dbtest.RandString()

		v, err := domain.NewEmailVerification(userID, time.Hour)
		require.NoError(t, err)
		issued, err := repo.Issue(ctx, v, tokenHash)
		require.NoError(t, err)
		require.NotEmpty(t, issued.ID(), "the adapter assigns an id")

		got, err := repo.GetByTokenHash(ctx, tokenHash)
		require.NoError(t, err)
		require.Equal(t, issued.ID(), got.ID())
		require.Equal(t, userID, got.UserID())
		require.WithinDuration(t, v.ExpiresAt(), got.ExpiresAt(), time.Second)
		require.False(t, got.IsConsumed())
		require.False(t, got.IsExpired(time.Now()))

		require.NoError(t, repo.Consume(ctx, got.ID(), time.Now()))
		consumed, err := repo.GetByTokenHash(ctx, tokenHash)
		require.NoError(t, err)
		require.True(t, consumed.IsConsumed(), "the row reflects the consumption")

		// Consuming an already-consumed challenge is a harmless no-op.
		require.NoError(t, repo.Consume(ctx, got.ID(), time.Now()))
	})
}

func TestEmailVerificationsIssueSupersedesPending(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.verifications(t)
		ctx := context.Background()
		userID := b.newUserID(t)
		firstHash, secondHash := dbtest.RandString(), dbtest.RandString()

		first, err := domain.NewEmailVerification(userID, time.Hour)
		require.NoError(t, err)
		_, err = repo.Issue(ctx, first, firstHash)
		require.NoError(t, err)

		second, err := domain.NewEmailVerification(userID, time.Hour)
		require.NoError(t, err)
		_, err = repo.Issue(ctx, second, secondHash)
		require.NoError(t, err)

		// Issuing a fresh challenge deletes the prior unconsumed one (FR-012).
		_, err = repo.GetByTokenHash(ctx, firstHash)
		require.ErrorIs(t, err, domain.ErrVerificationLinkInvalid)
		_, err = repo.GetByTokenHash(ctx, secondHash)
		require.NoError(t, err)
	})
}

func TestEmailVerificationsIssueKeepsConsumed(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.verifications(t)
		ctx := context.Background()
		userID := b.newUserID(t)
		consumedHash := dbtest.RandString()

		first, err := domain.NewEmailVerification(userID, time.Hour)
		require.NoError(t, err)
		issued, err := repo.Issue(ctx, first, consumedHash)
		require.NoError(t, err)
		require.NoError(t, repo.Consume(ctx, issued.ID(), time.Now()))

		second, err := domain.NewEmailVerification(userID, time.Hour)
		require.NoError(t, err)
		_, err = repo.Issue(ctx, second, dbtest.RandString())
		require.NoError(t, err)

		// Only unconsumed challenges are superseded: a consumed one is kept so a
		// reopened link can still report "already verified".
		got, err := repo.GetByTokenHash(ctx, consumedHash)
		require.NoError(t, err)
		require.True(t, got.IsConsumed())
	})
}

func TestEmailVerificationsGetUnknownToken(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		_, err := b.verifications(t).GetByTokenHash(context.Background(), dbtest.RandString())
		require.ErrorIs(t, err, domain.ErrVerificationLinkInvalid)
	})
}

// TestEmailVerificationsDeleteExpiredBefore is pg-only: DeleteExpiredBefore is
// not on the port. It backs the Postgres cleanup job, and YDB uses table TTL
// instead.
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

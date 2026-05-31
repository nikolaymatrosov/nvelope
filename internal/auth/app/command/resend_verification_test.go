package command_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/auth/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
)

func TestResendVerificationForUnverifiedAccount(t *testing.T) {
	t.Parallel()
	users := newFakeUsers()
	verifications := newFakeEmailVerifications()
	enqueuer := newFakeVerificationEnqueuer()
	userID := createTestUser(t, users, "ada@example.com")

	h := command.NewResendEmailVerificationHandler(users, verifications, enqueuer,
		fakeResendThrottle{allow: true}, time.Hour)
	err := h.Handle(context.Background(), command.ResendEmailVerification{Email: "ada@example.com"})
	require.NoError(t, err)

	require.Len(t, enqueuer.calls, 1, "a fresh verification email is enqueued")
	require.Equal(t, userID, enqueuer.calls[0].userID)
	require.Len(t, verifications.byHash, 1, "a fresh challenge is issued")
}

func TestResendVerificationSilentForAlreadyVerifiedAccount(t *testing.T) {
	t.Parallel()
	users := newFakeUsers()
	enqueuer := newFakeVerificationEnqueuer()
	userID := createTestUser(t, users, "ada@example.com")
	require.NoError(t, users.MarkEmailVerified(context.Background(), userID, time.Now()))

	h := command.NewResendEmailVerificationHandler(users, newFakeEmailVerifications(),
		enqueuer, fakeResendThrottle{allow: true}, time.Hour)
	err := h.Handle(context.Background(), command.ResendEmailVerification{Email: "ada@example.com"})
	require.NoError(t, err, "an already-verified account is a silent no-op")
	require.Empty(t, enqueuer.calls, "no email is sent to a verified account")
}

func TestResendVerificationSilentForUnknownEmail(t *testing.T) {
	t.Parallel()
	enqueuer := newFakeVerificationEnqueuer()

	h := command.NewResendEmailVerificationHandler(newFakeUsers(), newFakeEmailVerifications(),
		enqueuer, fakeResendThrottle{allow: true}, time.Hour)
	err := h.Handle(context.Background(), command.ResendEmailVerification{Email: "nobody@example.com"})
	require.NoError(t, err, "an unknown address is indistinguishable from a real one")
	require.Empty(t, enqueuer.calls)
}

func TestResendVerificationThrottled(t *testing.T) {
	t.Parallel()
	users := newFakeUsers()
	enqueuer := newFakeVerificationEnqueuer()
	createTestUser(t, users, "ada@example.com")

	h := command.NewResendEmailVerificationHandler(users, newFakeEmailVerifications(),
		enqueuer, fakeResendThrottle{allow: false}, time.Hour)
	err := h.Handle(context.Background(), command.ResendEmailVerification{Email: "ada@example.com"})
	require.ErrorIs(t, err, domain.ErrVerificationResendThrottled)
	require.Empty(t, enqueuer.calls, "a throttled request sends nothing")
}

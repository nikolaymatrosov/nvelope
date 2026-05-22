package command_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/auth/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// createTestUser inserts an unverified user into the fake and returns its id.
func createTestUser(t *testing.T, users *fakeUsers, email string) string {
	t.Helper()
	addr, err := domain.NewEmail(email)
	require.NoError(t, err)
	u, err := domain.NewUser(addr, "Test User")
	require.NoError(t, err)
	created, err := users.Create(context.Background(), u, "hash")
	require.NoError(t, err)
	return created.ID()
}

func TestVerifyEmailVerifiesValidToken(t *testing.T) {
	t.Parallel()
	users := newFakeUsers()
	verifications := newFakeEmailVerifications()
	userID := createTestUser(t, users, "ada@example.com")

	raw, err := token.New()
	require.NoError(t, err)
	verifications.byHash[token.Hash(raw)] = domain.HydrateEmailVerification(
		"v1", userID, time.Now().Add(time.Hour), time.Now(), nil)

	h := command.NewVerifyEmailHandler(users, verifications)
	result, err := h.Handle(context.Background(), command.VerifyEmail{Token: raw})
	require.NoError(t, err)
	require.False(t, result.AlreadyVerified)

	u, err := users.GetByID(context.Background(), userID)
	require.NoError(t, err)
	require.True(t, u.IsEmailVerified(), "the account is now verified")
	require.True(t, verifications.consumed["v1"], "the token is consumed")
}

func TestVerifyEmailIsIdempotentForConsumedToken(t *testing.T) {
	t.Parallel()
	users := newFakeUsers()
	verifications := newFakeEmailVerifications()
	userID := createTestUser(t, users, "ada@example.com")

	raw, err := token.New()
	require.NoError(t, err)
	consumedAt := time.Now().Add(-time.Minute)
	verifications.byHash[token.Hash(raw)] = domain.HydrateEmailVerification(
		"v1", userID, time.Now().Add(time.Hour), time.Now().Add(-time.Hour), &consumedAt)

	h := command.NewVerifyEmailHandler(users, verifications)
	result, err := h.Handle(context.Background(), command.VerifyEmail{Token: raw})
	require.NoError(t, err, "re-opening a used link is not an error")
	require.True(t, result.AlreadyVerified)
}

func TestVerifyEmailRejectsExpiredToken(t *testing.T) {
	t.Parallel()
	users := newFakeUsers()
	verifications := newFakeEmailVerifications()
	userID := createTestUser(t, users, "ada@example.com")

	raw, err := token.New()
	require.NoError(t, err)
	verifications.byHash[token.Hash(raw)] = domain.HydrateEmailVerification(
		"v1", userID, time.Now().Add(-time.Hour), time.Now().Add(-2*time.Hour), nil)

	h := command.NewVerifyEmailHandler(users, verifications)
	_, err = h.Handle(context.Background(), command.VerifyEmail{Token: raw})
	require.ErrorIs(t, err, domain.ErrVerificationLinkInvalid)
}

func TestVerifyEmailRejectsUnknownAndEmptyToken(t *testing.T) {
	t.Parallel()
	h := command.NewVerifyEmailHandler(newFakeUsers(), newFakeEmailVerifications())

	_, err := h.Handle(context.Background(), command.VerifyEmail{Token: "no-such-token"})
	require.ErrorIs(t, err, domain.ErrVerificationLinkInvalid)

	_, err = h.Handle(context.Background(), command.VerifyEmail{Token: ""})
	require.ErrorIs(t, err, domain.ErrVerificationLinkInvalid)
}

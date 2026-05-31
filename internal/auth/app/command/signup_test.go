package command_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/auth/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
)

// unrestricted is the open registration policy used by tests that are not
// exercising the domain allowlist.
var unrestricted = domain.NewRegistrationPolicy(nil)

func TestSignUpCreatesUnverifiedUserAndEnqueuesVerification(t *testing.T) {
	t.Parallel()
	users := newFakeUsers()
	enqueuer := newFakeVerificationEnqueuer()
	h := command.NewSignUpHandler(users, stubHasher{}, enqueuer, unrestricted, time.Hour)

	result, err := h.Handle(context.Background(), command.SignUp{
		Email: "ada@example.com", Password: "a-good-password", Name: "Ada",
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.UserID)
	require.Equal(t, "ada@example.com", result.UserEmail)
	require.Equal(t, "Ada", result.UserName)

	created, err := users.GetByID(context.Background(), result.UserID)
	require.NoError(t, err)
	require.False(t, created.IsEmailVerified(), "a new account starts unverified")

	require.Len(t, enqueuer.calls, 1, "exactly one verification email is enqueued")
	require.Equal(t, result.UserID, enqueuer.calls[0].userID)
	require.NotEmpty(t, enqueuer.calls[0].token, "the raw verification token rides the job")
}

func TestSignUpRejectsBadInput(t *testing.T) {
	t.Parallel()
	enqueuer := newFakeVerificationEnqueuer()
	h := command.NewSignUpHandler(newFakeUsers(), stubHasher{}, enqueuer, unrestricted, time.Hour)

	_, err := h.Handle(context.Background(), command.SignUp{
		Email: "not-an-email", Password: "a-good-password", Name: "Ada",
	})
	require.Error(t, err)

	_, err = h.Handle(context.Background(), command.SignUp{
		Email: "ada@example.com", Password: "short", Name: "Ada",
	})
	require.Error(t, err, "a short password is rejected")

	require.Empty(t, enqueuer.calls, "rejected registrations never enqueue a verification email")
}

func TestSignUpRejectsDuplicateEmail(t *testing.T) {
	t.Parallel()
	users := newFakeUsers()
	h := command.NewSignUpHandler(users, stubHasher{}, newFakeVerificationEnqueuer(),
		unrestricted, time.Hour)
	cmd := command.SignUp{Email: "ada@example.com", Password: "a-good-password", Name: "Ada"}

	_, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)

	_, err = h.Handle(context.Background(), cmd)
	require.ErrorIs(t, err, domain.ErrEmailTaken)
}

func TestSignUpRejectsDisallowedEmailDomain(t *testing.T) {
	t.Parallel()
	users := newFakeUsers()
	enqueuer := newFakeVerificationEnqueuer()
	policy := domain.NewRegistrationPolicy([]string{"example.com"})
	h := command.NewSignUpHandler(users, stubHasher{}, enqueuer, policy, time.Hour)

	_, err := h.Handle(context.Background(), command.SignUp{
		Email: "ada@other.com", Password: "a-good-password", Name: "Ada",
	})
	require.ErrorIs(t, err, domain.ErrEmailDomainNotAllowed)

	// No account was created and no verification email was enqueued (FR-015).
	_, err = users.LookupByEmail(context.Background(), "ada@other.com")
	require.ErrorIs(t, err, domain.ErrUserNotFound)
	require.Empty(t, enqueuer.calls)

	// A listed domain still registers.
	_, err = h.Handle(context.Background(), command.SignUp{
		Email: "ada@example.com", Password: "a-good-password", Name: "Ada",
	})
	require.NoError(t, err)
}

package command_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/auth/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
)

func TestSignUpCreatesUserAndToken(t *testing.T) {
	t.Parallel()
	h := command.NewSignUpHandler(newFakeUsers(), stubHasher{}, time.Hour)

	result, err := h.Handle(context.Background(), command.SignUp{
		Email: "ada@example.com", Password: "a-good-password", Name: "Ada",
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.UserID)
	require.Equal(t, "ada@example.com", result.UserEmail)
	require.Equal(t, "Ada", result.UserName)
	require.NotEmpty(t, result.Token, "a session token is surfaced once")
}

func TestSignUpRejectsBadInput(t *testing.T) {
	t.Parallel()
	h := command.NewSignUpHandler(newFakeUsers(), stubHasher{}, time.Hour)

	_, err := h.Handle(context.Background(), command.SignUp{
		Email: "not-an-email", Password: "a-good-password", Name: "Ada",
	})
	require.Error(t, err)

	_, err = h.Handle(context.Background(), command.SignUp{
		Email: "ada@example.com", Password: "short", Name: "Ada",
	})
	require.Error(t, err, "a short password is rejected")
}

func TestSignUpRejectsDuplicateEmail(t *testing.T) {
	t.Parallel()
	users := newFakeUsers()
	h := command.NewSignUpHandler(users, stubHasher{}, time.Hour)
	cmd := command.SignUp{Email: "ada@example.com", Password: "a-good-password", Name: "Ada"}

	_, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)

	_, err = h.Handle(context.Background(), cmd)
	require.ErrorIs(t, err, domain.ErrEmailTaken)
}

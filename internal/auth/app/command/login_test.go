package command_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/auth/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
)

func seedUser(t *testing.T, users *fakeUsers, email, password string) {
	t.Helper()
	addr, err := domain.NewEmail(email)
	require.NoError(t, err)
	u, err := domain.NewUser(addr, "Seeded User")
	require.NoError(t, err)
	_, err = users.Create(context.Background(), u, "hash:"+password)
	require.NoError(t, err)
}

func TestLogInSucceeds(t *testing.T) {
	t.Parallel()
	users := newFakeUsers()
	seedUser(t, users, "ada@example.com", "a-good-password")
	h := command.NewLogInHandler(users, newFakeSessions(), stubHasher{}, time.Hour)

	result, err := h.Handle(context.Background(), command.LogIn{
		Email: "ada@example.com", Password: "a-good-password",
	})
	require.NoError(t, err)
	require.Equal(t, "ada@example.com", result.UserEmail)
	require.NotEmpty(t, result.Token)
}

func TestLogInRejectsWrongPasswordAndUnknownEmailIdentically(t *testing.T) {
	t.Parallel()
	users := newFakeUsers()
	seedUser(t, users, "ada@example.com", "a-good-password")
	h := command.NewLogInHandler(users, newFakeSessions(), stubHasher{}, time.Hour)

	_, wrongPassword := h.Handle(context.Background(), command.LogIn{
		Email: "ada@example.com", Password: "the-wrong-password",
	})
	_, unknownEmail := h.Handle(context.Background(), command.LogIn{
		Email: "nobody@example.com", Password: "a-good-password",
	})

	require.ErrorIs(t, wrongPassword, domain.ErrInvalidCredentials)
	require.ErrorIs(t, unknownEmail, domain.ErrInvalidCredentials)
	require.Equal(t, wrongPassword.Error(), unknownEmail.Error(),
		"account enumeration is resisted — both cases are indistinguishable")
}

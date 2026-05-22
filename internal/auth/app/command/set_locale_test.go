package command_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/auth/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
)

func newStoredUser(t *testing.T, users *fakeUsers, email string) *domain.User {
	t.Helper()
	addr, err := domain.NewEmail(email)
	require.NoError(t, err)
	u, err := domain.NewUser(addr, "Test User")
	require.NoError(t, err)
	created, err := users.Create(context.Background(), u, "hash")
	require.NoError(t, err)
	return created
}

func TestSetLocalePersistsSupportedLocale(t *testing.T) {
	t.Parallel()
	users := newFakeUsers()
	created := newStoredUser(t, users, "ada@example.com")

	h := command.NewSetLocaleHandler(users)
	require.NoError(t, h.Handle(context.Background(),
		command.SetLocale{UserID: created.ID(), Locale: "ru"}))

	got, err := users.GetByID(context.Background(), created.ID())
	require.NoError(t, err)
	require.Equal(t, "ru", got.Locale().String())
}

func TestSetLocaleRejectsUnsupportedLocale(t *testing.T) {
	t.Parallel()
	users := newFakeUsers()
	created := newStoredUser(t, users, "ada@example.com")

	h := command.NewSetLocaleHandler(users)
	err := h.Handle(context.Background(),
		command.SetLocale{UserID: created.ID(), Locale: "de"})
	require.Error(t, err, "an unsupported locale is rejected")
}

func TestSetLocaleUnknownUser(t *testing.T) {
	t.Parallel()
	users := newFakeUsers()

	h := command.NewSetLocaleHandler(users)
	err := h.Handle(context.Background(),
		command.SetLocale{UserID: "user-missing", Locale: "ru"})
	require.ErrorIs(t, err, domain.ErrUserNotFound)
}

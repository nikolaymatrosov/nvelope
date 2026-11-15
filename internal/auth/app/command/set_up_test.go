package command_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/auth/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
)

const setupToken = "0123456789abcdef0123456789abcdef"

func TestSetUpCreatesVerifiedSignedInOwner(t *testing.T) {
	t.Parallel()
	users := newFakeUsers()
	h := command.NewSetUpHandler(users, stubHasher{}, setupToken, time.Hour)

	result, err := h.Handle(context.Background(), command.SetUp{
		Token: setupToken, Email: "owner@example.com", Password: "a-good-password", Name: "Owner",
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.UserID)
	require.Equal(t, "owner@example.com", result.UserEmail)
	require.Equal(t, "Owner", result.UserName)
	require.NotEmpty(t, result.SessionToken, "setup signs the owner in")

	created, err := users.GetByID(context.Background(), result.UserID)
	require.NoError(t, err)
	require.True(t, created.IsEmailVerified(), "holding the setup token proves the right to install")
}

func TestSetUpRejectsWrongToken(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ configured, given string }{
		"missing":           {setupToken, ""},
		"wrong":             {setupToken, "not-the-token"},
		"longer":            {setupToken, setupToken + "x"},
		"setup not enabled": {"", ""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			users := newFakeUsers()
			h := command.NewSetUpHandler(users, stubHasher{}, tc.configured, time.Hour)

			_, err := h.Handle(context.Background(), command.SetUp{
				Token: tc.given, Email: "owner@example.com", Password: "a-good-password", Name: "Owner",
			})
			require.ErrorIs(t, err, domain.ErrSetupTokenInvalid)
			_, err = users.LookupByEmail(context.Background(), "owner@example.com")
			require.ErrorIs(t, err, domain.ErrUserNotFound, "a refused setup creates nothing")
		})
	}
}

func TestSetUpRejectsBadInput(t *testing.T) {
	t.Parallel()
	users := newFakeUsers()
	h := command.NewSetUpHandler(users, stubHasher{}, setupToken, time.Hour)

	_, err := h.Handle(context.Background(), command.SetUp{
		Token: setupToken, Email: "owner@example.com", Password: "short", Name: "Owner",
	})
	require.Error(t, err)
	_, err = users.LookupByEmail(context.Background(), "owner@example.com")
	require.ErrorIs(t, err, domain.ErrUserNotFound)
}

package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

func TestNewEmailNormalizesAndValidates(t *testing.T) {
	t.Parallel()

	e, err := domain.NewEmail("  ada@example.com  ")
	require.NoError(t, err)
	require.Equal(t, "ada@example.com", e.String(), "surrounding whitespace is trimmed")
	require.False(t, e.IsZero())
}

func TestNewEmailRejectsBadShapes(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"", "ada", "ada@", "@example.com", "ada@example", "a b@example.com"} {
		_, err := domain.NewEmail(raw)
		require.Error(t, err, "%q is not a valid address", raw)
		ae, ok := apperr.As(err)
		require.True(t, ok)
		require.Equal(t, "validation_failed", ae.Slug())
		require.Equal(t, apperr.IncorrectInput, ae.Category())
	}
}

func TestNewPasswordEnforcesLengthBounds(t *testing.T) {
	t.Parallel()

	_, err := domain.NewPassword("short")
	require.Error(t, err, "a 5-character password is too short")

	_, err = domain.NewPassword("12345678")
	require.NoError(t, err, "8 characters is the minimum")

	long := make([]byte, 73)
	for i := range long {
		long[i] = 'a'
	}
	_, err = domain.NewPassword(string(long))
	require.Error(t, err, "73 bytes exceeds bcrypt's limit")

	p, err := domain.NewPassword("a-good-password")
	require.NoError(t, err)
	require.Equal(t, "a-good-password", p.String())
}

func TestNewUserValidatesNameAndEmail(t *testing.T) {
	t.Parallel()

	email, err := domain.NewEmail("ada@example.com")
	require.NoError(t, err)

	u, err := domain.NewUser(email, "  Ada Lovelace  ")
	require.NoError(t, err)
	require.Equal(t, "Ada Lovelace", u.Name(), "the name is trimmed")
	require.Equal(t, "ada@example.com", u.Email().String())
	require.Empty(t, u.ID(), "a new user has no id until persisted")

	_, err = domain.NewUser(email, "   ")
	require.Error(t, err, "an empty name is rejected")

	_, err = domain.NewUser(domain.Email{}, "Ada")
	require.Error(t, err, "the zero-value email is rejected")
}

func TestHydrateUserSkipsValidation(t *testing.T) {
	t.Parallel()

	u := domain.HydrateUser("user-123", "grace@example.com", "Grace")
	require.Equal(t, "user-123", u.ID())
	require.Equal(t, "grace@example.com", u.Email().String())
	require.Equal(t, "Grace", u.Name())
}

package apperr_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

func TestCategoryConstructors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		err      *apperr.Error
		category apperr.Category
	}{
		{"incorrect input", apperr.NewIncorrectInput("validation_failed", "bad"), apperr.IncorrectInput},
		{"conflict", apperr.NewConflict("email_taken", "taken"), apperr.Conflict},
		{"not found", apperr.NewNotFound("tenant_not_found", "gone"), apperr.NotFound},
		{"authorization", apperr.NewAuthorization("unauthenticated", "nope"), apperr.Authorization},
		{"forbidden", apperr.NewForbidden("forbidden-lists-manage", "nope"), apperr.Forbidden},
		{"unknown", apperr.NewUnknown("internal_error", "oops"), apperr.Unknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.category, tc.err.Category())
		})
	}
}

func TestCategoryString(t *testing.T) {
	t.Parallel()
	require.Equal(t, "forbidden", apperr.Forbidden.String())
	require.Equal(t, "authorization", apperr.Authorization.String())
}

func TestSlugAndMessageRoundTrip(t *testing.T) {
	t.Parallel()
	err := apperr.NewConflict("slug_taken", "that workspace address is already in use")
	require.Equal(t, "slug_taken", err.Slug())
	require.Equal(t, "that workspace address is already in use", err.Message())
}

func TestAsExtractsThroughWrapping(t *testing.T) {
	t.Parallel()
	base := apperr.NewNotFound("invitation_not_found", "this invitation is not valid")
	wrapped := fmt.Errorf("accepting invitation: %w", base)

	extracted, ok := apperr.As(wrapped)
	require.True(t, ok)
	require.Equal(t, "invitation_not_found", extracted.Slug())
	require.Equal(t, apperr.NotFound, extracted.Category())

	_, ok = apperr.As(errors.New("plain error"))
	require.False(t, ok)
}

func TestIsMatchesBySlug(t *testing.T) {
	t.Parallel()
	sentinel := apperr.NewConflict("email_taken", "that email is already registered")
	raised := apperr.NewConflict("email_taken", "a different message, same slug")

	require.ErrorIs(t, raised, sentinel, "errors with the same slug compare equal")
	require.ErrorIs(t, fmt.Errorf("signup: %w", raised), sentinel, "match survives wrapping")
	require.NotErrorIs(t, apperr.NewConflict("slug_taken", "x"), sentinel)
}

func TestWrapCarriesCause(t *testing.T) {
	t.Parallel()
	cause := errors.New("connection reset")
	err := apperr.Wrap(cause, apperr.Unknown, "internal_error", "something went wrong")

	require.Equal(t, cause, errors.Unwrap(err))
	require.ErrorIs(t, err, cause)
	require.Equal(t, apperr.Unknown, err.Category())
}

func TestWithMessagePreservesSlugAndCategory(t *testing.T) {
	t.Parallel()
	base := apperr.NewConflict("email_taken", "that email is already registered")
	custom := base.WithMessage("an account with this email already exists — log in first")

	require.Equal(t, "email_taken", custom.Slug())
	require.Equal(t, apperr.Conflict, custom.Category())
	require.Equal(t, "an account with this email already exists — log in first", custom.Message())
	require.ErrorIs(t, custom, base, "the message override still matches the sentinel")
}

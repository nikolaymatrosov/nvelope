package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	deliverabilitydomain "github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

func TestStatusForCategory(t *testing.T) {
	t.Parallel()
	cases := []struct {
		category apperr.Category
		want     int
	}{
		{apperr.IncorrectInput, http.StatusUnprocessableEntity},
		{apperr.Conflict, http.StatusConflict},
		{apperr.NotFound, http.StatusNotFound},
		{apperr.Authorization, http.StatusUnauthorized},
		{apperr.Forbidden, http.StatusForbidden},
		{apperr.Unknown, http.StatusInternalServerError},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, statusForCategory(tc.category))
	}
}

func TestDeliverabilityErrorSlugsMapToExpectedStatus(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err  error
		slug string
		want int
	}{
		{deliverabilitydomain.ErrSuppressionNotFound, "suppression_not_found", http.StatusNotFound},
		{deliverabilitydomain.ErrRecipientSuppressed, "recipient_suppressed", http.StatusConflict},
		{deliverabilitydomain.ErrValidationFailed, "validation_failed", http.StatusUnprocessableEntity},
	}
	for _, tc := range cases {
		ae, ok := apperr.As(tc.err)
		require.True(t, ok)
		require.Equal(t, tc.slug, ae.Slug())
		require.Equal(t, tc.want, statusForCategory(ae.Category()))
	}
}

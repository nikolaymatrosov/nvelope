package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

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

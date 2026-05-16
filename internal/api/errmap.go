package api

import (
	"net/http"

	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

// statusForCategory maps a transport-agnostic error category to an HTTP status
// code. This is the only place that mapping lives.
func statusForCategory(c apperr.Category) int {
	switch c {
	case apperr.IncorrectInput:
		return http.StatusUnprocessableEntity
	case apperr.Conflict:
		return http.StatusConflict
	case apperr.NotFound:
		return http.StatusNotFound
	case apperr.Authorization:
		return http.StatusUnauthorized
	default:
		return http.StatusInternalServerError
	}
}

// fail is the single place a domain error becomes an HTTP response. A typed
// apperr is mapped by its category to a status code and rendered with its
// stable slug. Any other error — or an apperr of unknown category — is an
// unexpected internal failure: it is logged and returned as a generic 500.
func (s *Server) fail(w http.ResponseWriter, op string, err error) {
	if ae, ok := apperr.As(err); ok && ae.Category() != apperr.Unknown {
		writeError(w, statusForCategory(ae.Category()), ae.Slug(), ae.Message())
		return
	}
	s.logger.Error("request failed", "op", op, "error", err)
	writeError(w, http.StatusInternalServerError, "internal_error", "something went wrong")
}

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
	case apperr.Forbidden:
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError
	}
}

// statusForSlug overrides the category-derived status for a few error slugs
// whose HTTP semantics are more specific than their category. An external
// provider failure is an Unknown-category error, but the API contract
// distinguishes a bad gateway (the provider rejected the request) from a
// generic internal error.
var statusForSlug = map[string]int{
	"provisioning-failed": http.StatusBadGateway,
	// A declined payment is a distinct, well-known HTTP status — neither a
	// generic 422 nor a 500.
	"payment_failed": http.StatusPaymentRequired,
	// A throttled verification-email resend is a 429, not a generic 422.
	"verification_resend_throttled": http.StatusTooManyRequests,
	// A request body over its size limit is a 413, not a generic 422 — the
	// same status and slug limitImportUploadSize's http.MaxBytesReader path
	// renders (openapi_wiring.go's validationErrorHandler), so StartImport's
	// own oversize-file check (audience_api.go) answers identically.
	"payload_too_large": http.StatusRequestEntityTooLarge,
}

// fail is the single place a domain error becomes an HTTP response. A typed
// apperr is mapped by its category to a status code and rendered with its
// stable slug; a handful of slugs carry a more specific status. Any other
// error is an unexpected internal failure: logged, then rendered as a generic
// 500 problem document.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, op string, err error) {
	if ae, ok := apperr.As(err); ok {
		if status, override := statusForSlug[ae.Slug()]; override {
			writeProblem(w, r, status, ae.Slug(), ae.Message(), nil)
			return
		}
		if ae.Category() != apperr.Unknown {
			writeProblem(w, r, statusForCategory(ae.Category()), ae.Slug(), ae.Message(), nil)
			return
		}
	}
	if s.logger != nil {
		s.logger.Error("request failed", "op", op, "error", err)
	}
	writeProblem(w, r, http.StatusInternalServerError, "internal_error", "something went wrong", nil)
}

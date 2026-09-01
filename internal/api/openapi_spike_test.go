package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	oapimiddleware "github.com/oapi-codegen/nethttp-middleware"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/api/openapitenant"
)

// TestValidatorMatchesTemplatedServerBase is the load-bearing assumption of the
// whole spec-first layout: the request validator must match a real request path
// (/t/acme/api/lists) against a servers: entry templated as /t/{slug}/api.
//
// If this fails, the fallback documented in the design spec is to drop servers:
// templating and write full paths in tenant.yaml.
func TestValidatorMatchesTemplatedServerBase(t *testing.T) {
	t.Parallel()
	// GetSpec, not the deprecated GetSwagger wrapper: oapi-codegen v2.8.0
	// renamed it after kin-openapi renamed openapi3.Swagger to openapi3.T.
	swagger, err := openapitenant.GetSpec()
	require.NoError(t, err)

	validator := oapimiddleware.OapiRequestValidator(swagger)

	ok := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"items":[],"total":0}`))
	}

	r := chi.NewRouter()
	r.Route("/t/{slug}/api", func(tr chi.Router) {
		tr.With(validator).Get("/lists", ok)
		// Mounted in the same subtree but absent from the spec, so the
		// validator must reject it.
		tr.With(validator).Get("/not-in-the-spec", ok)
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/t/acme/api/lists", nil))

	require.Equal(t, http.StatusOK, rec.Code,
		"the validator resolved /t/acme/api/lists against the templated server base")

	// Without this, the assertion above would look identical if the validator
	// had never been attached, or if it passed every request through. A 404
	// here proves the 200 above is a real match against the templated base.
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/t/acme/api/not-in-the-spec", nil))

	require.Equal(t, http.StatusNotFound, rec.Code,
		"the validator rejected a path that the router serves but the spec does not declare")
}

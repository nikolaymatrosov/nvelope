package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/go-chi/chi/v5"
	oapimiddleware "github.com/oapi-codegen/nethttp-middleware"

	"github.com/nikolaymatrosov/nvelope/internal/api/openapitenant"
)

// mountGeneratedTenantAPI registers every operation described by
// api/openapi/tenant.yaml onto r, which must already carry the tenant plane's
// authentication, tenant-resolution, and authz middleware.
//
// The request validator runs only on generated routes (it is passed through
// ChiServerOptions.Middlewares, not r.Use), because unmigrated routes are not
// in the spec and the validator would reject them with a 404.
//
// A path is served here if and only if it appears under `paths:` in
// api/openapi/tenant.yaml. A route may not be registered both here and by hand
// in Handler — chi panics on a duplicate pattern — so migrating an operation
// means adding it to the spec and deleting the hand-written line in the same
// change.
func (s *Server) mountGeneratedTenantAPI(r chi.Router) {
	// GetSpec, not the deprecated GetSwagger wrapper: oapi-codegen renamed it
	// after kin-openapi renamed openapi3.Swagger to openapi3.T.
	spec, err := openapitenant.GetSpec()
	if err != nil {
		// The spec is embedded at build time; a parse failure is a programming
		// error, not a runtime condition.
		panic("loading embedded tenant OpenAPI document: " + err.Error())
	}

	strict := openapitenant.NewStrictHandlerWithOptions(s, nil, openapitenant.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  s.strictRequestError,
		ResponseErrorHandlerFunc: s.strictResponseError,
	})

	openapitenant.HandlerWithOptions(strict, openapitenant.ChiServerOptions{
		BaseRouter:  r,
		Middlewares: []openapitenant.MiddlewareFunc{s.openapiRequestValidator(spec)},
		ErrorHandlerFunc: func(w http.ResponseWriter, req *http.Request, err error) {
			// Raised by the generated wrapper when a path or query parameter
			// cannot be bound to its Go type at all.
			writeProblem(w, req, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		},
	})
}

// openapiRequestValidator builds the kin-openapi request-validation middleware
// used on every generated route. It is a method taking the spec, rather than
// reading the embedded document itself, so a test can build a validator with
// exactly the production configuration over a document it controls.
func (s *Server) openapiRequestValidator(spec *openapi3.T) openapitenant.MiddlewareFunc {
	return oapimiddleware.OapiRequestValidatorWithOptions(spec, &oapimiddleware.Options{
		Options: openapi3filter.Options{
			// tenant.yaml declares a document-level `security` requirement for
			// the nv_workspace cookie. kin-openapi enforces declared security
			// and rejects every request with 401 "missing AuthenticationFunc"
			// unless one is supplied — but authentication is chi middleware's
			// job (requireUser → resolveTenant → authz), and it has already run
			// by the time this validator sees the request. The validator can
			// only see that a cookie exists, never that its session is valid,
			// so letting it decide would be both redundant and wrong. The noop
			// func says "authentication is somebody else's problem", not
			// "there is no authentication".
			AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,

			// String-format validation (`format: email` on
			// CreateSubscriberRequest.email) is deliberately left off, which is
			// kin-openapi's default. A malformed address is rejected by the
			// audience domain as a 422 with a domain error code; turning format
			// checking on here would turn that into an anonymous 400 from the
			// middleware and move the rule out of the domain.
		},

		// The warning fires because `servers:` is non-nil and warns that Host
		// validation may reject valid requests. It does not apply here: the
		// single server entry is the relative, templated base /t/{slug}/api,
		// which carries no host — the validator matches it as a path prefix.
		// DoNotValidateServers is deliberately NOT set: it works by nilling
		// spec.Servers, which would strip that base and stop /t/acme/api/lists
		// from matching /lists at all.
		SilenceServersWarning: true,

		ErrorHandlerWithOpts: s.validationErrorHandler,
	})
}

// validationErrorHandler renders a request-validation failure as a problem
// document with a populated errors array.
// It switches on the status the middleware suggests, because not every
// rejection is a validation failure: the middleware also refuses requests it
// could not route or authenticate, and rendering those as "validation_failed"
// would be a lie.
func (s *Server) validationErrorHandler(_ context.Context, err error, w http.ResponseWriter,
	r *http.Request, opts oapimiddleware.ErrorHandlerOpts) {
	switch opts.StatusCode {
	case http.StatusBadRequest:
		// The only status the middleware assigns to a real body or parameter
		// violation. The hand-written handlers answer a rejected body with 422,
		// and the contract keeps that.
		writeProblem(w, r, http.StatusUnprocessableEntity, "validation_failed",
			"the request body or parameters failed validation",
			fieldErrorsFrom(err))

	case http.StatusNotFound:
		// The document does not declare this path (the middleware reports a
		// routing miss as 404 whatever the method). Unreachable in production,
		// where the generated router registers exactly the spec's paths, but it
		// keeps the rendering honest if the two ever drift.
		writeProblem(w, r, http.StatusNotFound, "not_found",
			"no operation matches this request", nil)

	case http.StatusUnauthorized:
		// A declared security requirement was refused. Unreachable while
		// openapiRequestValidator installs NoopAuthenticationFunc — see the
		// comment there — but it must never be mislabelled as a body problem.
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated",
			"a workspace session is required", nil)

	default:
		// Anything else is the middleware's own "error validating route", which
		// is a bug on our side: log it and render a generic 500.
		s.fail(w, r, "openapi request validation", err)
	}
}

// strictRequestError fires when the generated code cannot bind a request into
// its typed request object — a malformed body that slipped past the validator.
func (s *Server) strictRequestError(w http.ResponseWriter, r *http.Request, err error) {
	writeProblem(w, r, http.StatusBadRequest, "invalid_body", err.Error(), nil)
}

// strictResponseError is the single exit for every error a strict handler
// returns. It reuses fail(), so an authz denial, a domain conflict, and an
// unexpected internal failure all render identical problem documents.
func (s *Server) strictResponseError(w http.ResponseWriter, r *http.Request, err error) {
	s.fail(w, r, "openapi handler", err)
}

// fieldErrorsFrom turns a kin-openapi validation failure into the Problem's
// errors array. A body schema violation carries a JSON Pointer to the offending
// property and the keyword that rejected it ("required", "enum", "maxLength"),
// which is exactly what a form needs to highlight the right input; a parameter
// violation names the parameter instead.
//
// The slice always holds exactly one entry. kin-openapi stops at the first
// violation unless openapi3filter.Options.MultiError is set, so claiming to be
// exhaustive would be a lie — the array shape is what the contract fixes, and
// multi-error reporting can be turned on later without a wire change. Anything
// unrecognised degrades to a single unlocated entry, so the array is never
// empty.
func fieldErrorsFrom(err error) []fieldError {
	var reqErr *openapi3filter.RequestError
	if !errors.As(err, &reqErr) {
		return []fieldError{{Field: "", Code: "invalid", Detail: err.Error()}}
	}

	// A parameter violation has no pointer into the body; the parameter's name
	// is the only location there is.
	field := ""
	if reqErr.Parameter != nil {
		field = reqErr.Parameter.Name
	}

	var schemaErr *openapi3.SchemaError
	if errors.As(reqErr.Err, &schemaErr) {
		// An empty pointer means the whole body failed (an unknown property
		// under additionalProperties: false reports against the object, not the
		// property), which RFC 6901 spells as the empty string.
		if p := schemaErr.JSONPointer(); len(p) > 0 {
			field = "/" + strings.Join(p, "/")
		}
		return []fieldError{{Field: field, Code: schemaErr.SchemaField, Detail: schemaErr.Reason}}
	}

	// RequestError.Error() embeds the whole rejected schema; Reason is the one
	// line a caller can act on.
	detail := reqErr.Reason
	if detail == "" {
		detail = reqErr.Error()
	}
	return []fieldError{{Field: field, Code: "invalid", Detail: detail}}
}

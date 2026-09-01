package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"unicode"
)

// problemBaseURI namespaces the RFC 9457 `type` URIs. The suffix is always the
// stable error code, so `type` and `code` never disagree.
const problemBaseURI = "https://nvelope.dev/probs/"

// problemContentType is the media type RFC 9457 mandates. Clients branch on the
// body, but the header is what makes a Problem self-describing to proxies and
// API tooling.
const problemContentType = "application/problem+json"

// fieldError is a Problem extension member describing one request-body
// violation. Field is a JSON Pointer into the submitted body.
type fieldError struct {
	Field  string `json:"field"`
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
}

// problem is an RFC 9457 problem detail. `code`, `requestId`, and `errors` are
// extension members: `code` preserves the flat apperr slug taxonomy so callers
// need not parse `type`, `requestId` makes a user-reported failure traceable,
// and `errors` carries field-level validation detail.
type problem struct {
	Type      string       `json:"type"`
	Title     string       `json:"title"`
	Status    int          `json:"status"`
	Detail    string       `json:"detail,omitempty"`
	Instance  string       `json:"instance,omitempty"`
	Code      string       `json:"code"`
	RequestID string       `json:"requestId,omitempty"`
	Errors    []fieldError `json:"errors,omitempty"`
}

// writeProblem renders an RFC 9457 document. It is the only function in the
// package that writes an error body.
func writeProblem(w http.ResponseWriter, r *http.Request, status int, code, detail string, fields []fieldError) {
	doc := problem{
		Type:   problemBaseURI + code,
		Title:  titleForCode(code),
		Status: status,
		Detail: detail,
		Code:   code,
		Errors: fields,
	}
	if r != nil {
		doc.Instance = r.URL.Path
		doc.RequestID = requestIDFromContext(r.Context())
	}
	w.Header().Set("Content-Type", problemContentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(doc)
}

// titleForCode derives a short human-readable title from an error code.
// Codes are either snake_case ("list_not_found") or kebab-case
// ("forbidden-lists-manage"); both become sentence case.
func titleForCode(code string) string {
	if code == "" {
		return "Error"
	}
	words := strings.FieldsFunc(code, func(r rune) bool { return r == '_' || r == '-' })
	if len(words) == 0 {
		return "Error"
	}
	joined := strings.Join(words, " ")
	runes := []rune(joined)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// withRequestIDForTest seeds a request id without running the middleware. It
// lives in the non-test file because requestIDCtxKey is unexported and tests in
// other files need it too.
func withRequestIDForTest(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDCtxKey, id)
}

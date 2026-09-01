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
//
// Extensions holds further, endpoint-specific extension members (RFC 9457
// §3.2 permits arbitrary additional members on a problem document). They are
// flattened into the top-level JSON object by MarshalJSON, not nested under
// an "extensions" key. A key that collides with one of the fixed members
// above is dropped — the standard member always wins, deterministically —
// see problemReservedKeys.
type problem struct {
	Type       string         `json:"type"`
	Title      string         `json:"title"`
	Status     int            `json:"status"`
	Detail     string         `json:"detail,omitempty"`
	Instance   string         `json:"instance,omitempty"`
	Code       string         `json:"code"`
	RequestID  string         `json:"requestId,omitempty"`
	Errors     []fieldError   `json:"errors,omitempty"`
	Extensions map[string]any `json:"-"`
}

// problemReservedKeys are the fixed problem members. An Extensions entry
// under one of these keys is silently dropped by MarshalJSON rather than
// overriding the standard member — see problem.Extensions.
var problemReservedKeys = map[string]struct{}{
	"type": {}, "title": {}, "status": {}, "detail": {}, "instance": {},
	"code": {}, "requestId": {}, "errors": {},
}

// MarshalJSON renders the fixed members via the ordinary struct encoding,
// then merges Extensions in as additional top-level keys. Reserved keys in
// Extensions are dropped (see problemReservedKeys) so a caller can never
// accidentally — or a malicious extension value can never — shadow a
// standard member.
func (p problem) MarshalJSON() ([]byte, error) {
	type alias problem
	base, err := json.Marshal(alias(p))
	if err != nil {
		return nil, err
	}
	if len(p.Extensions) == 0 {
		return base, nil
	}
	merged := make(map[string]any, len(p.Extensions)+8)
	if err := json.Unmarshal(base, &merged); err != nil {
		return nil, err
	}
	for k, v := range p.Extensions {
		if _, reserved := problemReservedKeys[k]; reserved {
			continue
		}
		merged[k] = v
	}
	return json.Marshal(merged)
}

// writeProblem renders an RFC 9457 document with no extension members. It
// delegates to writeProblemWithExtensions, the only function in the package
// that actually writes an error body.
func writeProblem(w http.ResponseWriter, r *http.Request, status int, code, detail string, fields []fieldError) {
	writeProblemWithExtensions(w, r, status, code, detail, fields, nil)
}

// writeProblemWithExtensions renders an RFC 9457 document with additional,
// endpoint-specific top-level members (e.g. the FR-009 stale-row conflict's
// currentUpdatedAt). It is the only function in the package that writes an
// error body; writeProblem and writeError both delegate to it.
func writeProblemWithExtensions(w http.ResponseWriter, r *http.Request, status int, code, detail string, fields []fieldError, ext map[string]any) {
	doc := problem{
		Type:       problemBaseURI + code,
		Title:      titleForCode(code),
		Status:     status,
		Detail:     detail,
		Code:       code,
		Errors:     fields,
		Extensions: ext,
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

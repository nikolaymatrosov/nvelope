// Package api wires the nvelope HTTP layer: the router, middleware, and
// request handlers for the platform and tenant-scoped APIs.
package api

import (
	"encoding/json"
	"net/http"
)

// writeJSON writes v as a JSON response with the given status code. A nil v
// writes only the status (used for 204 responses).
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// writeError writes an RFC 9457 problem document. The code is a stable
// machine-readable token; the message becomes the problem's `detail`.
func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeProblem(w, r, status, code, message, nil)
}

// decodeJSON reads a JSON request body into v, rejecting unknown fields and
// absent or malformed bodies.
func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// userPayload builds the JSON object for a platform user. An empty locale —
// the user has never chosen an interface language — is emitted as JSON null.
func userPayload(id, email, name, locale string) map[string]any {
	var loc any
	if locale != "" {
		loc = locale
	}
	return map[string]any{"id": id, "email": email, "name": name, "locale": loc}
}

// tenantPayload builds the JSON object for a workspace.
func tenantPayload(id, slug, name, status string) map[string]string {
	return map[string]string{"id": id, "slug": slug, "name": name, "status": status}
}

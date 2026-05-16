// Package api wires the nvelope HTTP layer: the router, middleware, and
// request handlers for the platform and tenant-scoped APIs.
package api

import (
	"encoding/json"
	"net/http"
)

// errorBody is the response envelope for every API error.
type errorBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// writeJSON writes v as a JSON response with the given status code. A nil v
// writes only the status (used for 204 responses).
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// writeError writes a {error, message} envelope with the given status code.
// The code is a stable machine-readable token; the message is human-readable.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Error: code, Message: message})
}

// decodeJSON reads a JSON request body into v, rejecting unknown fields and
// absent or malformed bodies.
func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

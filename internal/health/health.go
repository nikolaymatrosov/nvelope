// Package health serves the API service's liveness/readiness endpoint.
package health

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
)

// Handler answers health probes. It reports 200 once SetReady(true) has been
// called and 503 otherwise (while starting or draining). Safe for concurrent
// use.
type Handler struct {
	service string
	version string
	ready   atomic.Bool
}

// NewHandler returns a Handler that reports the given service and version.
// It starts in the not-ready state.
func NewHandler(service, version string) *Handler {
	return &Handler{service: service, version: version}
}

// SetReady marks the service ready (true) or draining/starting (false).
func (h *Handler) SetReady(ready bool) { h.ready.Store(ready) }

func (h *Handler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	status, code := "ok", http.StatusOK
	if !h.ready.Load() {
		status, code = "unavailable", http.StatusServiceUnavailable
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  status,
		"service": h.service,
		"version": h.version,
	})
}

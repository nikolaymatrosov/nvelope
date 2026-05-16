package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHandlerReportsUnavailableUntilReady(t *testing.T) {
	h := NewHandler("api", "v0.0.1")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "unavailable", body["status"])
	require.Equal(t, "api", body["service"])
	require.Equal(t, "v0.0.1", body["version"])
}

func TestHandlerReportsOKWhenReady(t *testing.T) {
	h := NewHandler("api", "v0.0.1")
	h.SetReady(true)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "ok", body["status"])
	require.Equal(t, "api", body["service"])
}

func TestHandlerReturnsToUnavailableWhenDraining(t *testing.T) {
	h := NewHandler("api", "v0.0.1")
	h.SetReady(true)
	h.SetReady(false)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

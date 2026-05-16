package logging

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewEmitsStructuredJSON(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, "api", "info")

	logger.Info("service starting")

	var line map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &line))
	require.Contains(t, line, "time")
	require.Equal(t, "INFO", line["level"])
	require.Equal(t, "service starting", line["msg"])
	require.Equal(t, "api", line["service"])
}

func TestNewRespectsLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, "worker", "warn")

	logger.Info("filtered out")
	require.Empty(t, buf.String(), "info line must be filtered when level is warn")

	logger.Warn("kept")
	require.NotEmpty(t, buf.String())
}

func TestNewUnknownLevelFallsBackToInfo(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, "scheduler", "verbose")

	logger.Info("kept at info")
	require.NotEmpty(t, buf.String())
}

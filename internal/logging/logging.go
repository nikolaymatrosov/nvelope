// Package logging builds the structured logger shared by every nvelope
// service. Output is JSON to the given writer, with a service attribute
// attached to every line.
package logging

import (
	"io"
	"log/slog"
	"strings"
)

// New returns a JSON logger tagged with the given service name and filtered
// to the given level. An unrecognized level falls back to info.
func New(w io.Writer, service, level string) *slog.Logger {
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: parseLevel(level)})
	return slog.New(handler).With(slog.String("service", service))
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

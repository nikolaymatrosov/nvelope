package domain

import (
	"bytes"
	"encoding/json"
)

// normalizeRawJSON returns nil for an absent, empty, or explicit-null jsonb
// payload and the trimmed copy otherwise. The aggregate hands these bytes to
// the read view verbatim, so encoding-quirky inputs (a single literal `null`,
// the empty slice the pgx driver yields for a SQL NULL) collapse to the same
// "no document" sentinel.
func normalizeRawJSON(b json.RawMessage) json.RawMessage {
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	out := make(json.RawMessage, len(trimmed))
	copy(out, trimmed)
	return out
}

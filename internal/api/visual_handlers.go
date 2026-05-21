package api

import (
	"bytes"
	"encoding/json"
	"net/http"

	campaigndomain "github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// decodeVisualPayload decodes the visual-save shape — a typed VisualDoc —
// out of the raw wire bytes. Shared by the campaign and template visual-
// save handlers (T034, T073). On a missing-or-malformed payload it writes
// the standard 400 invalid_body error and returns ok=false; callers must
// short-circuit without further response writes.
//
// The Theme override flows through verbatim as the raw bytes the caller
// passed in — Theme has only unexported fields, so there is nothing to
// decode against the typed struct, and the aggregate's pinned-theme slot
// is set from the JSON bytes directly. The raw bytes themselves continue
// to reach persistence alongside the decoded doc.
func (s *Server) decodeVisualPayload(w http.ResponseWriter,
	bodyDocRaw, themeRaw json.RawMessage,
) (*campaigndomain.VisualDoc, *campaigndomain.Theme, bool) {

	if !hasJSONValue(bodyDocRaw) {
		writeError(w, http.StatusBadRequest, "invalid_body", "bodyDoc is required")
		return nil, nil, false
	}
	var doc campaigndomain.VisualDoc
	if err := json.Unmarshal(bodyDocRaw, &doc); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "bodyDoc is not a valid visual document")
		return nil, nil, false
	}
	_ = themeRaw // Theme bytes flow through via the request struct, not here.
	return &doc, nil, true
}

// hasJSONValue reports whether raw carries a non-null, non-empty JSON
// payload — the wire-shape parallel to the domain's normalizeRawJSON.
func hasJSONValue(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null"))
}

package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// transparentGIF is a 1×1 fully transparent GIF — the open-tracking pixel.
var transparentGIF = []byte{
	0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00, 0x80, 0x00,
	0x00, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0x21, 0xf9, 0x04, 0x01, 0x00,
	0x00, 0x00, 0x00, 0x2c, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00,
	0x00, 0x02, 0x02, 0x44, 0x01, 0x00, 0x3b,
}

// handleTrackOpen records a campaign open and returns the tracking pixel. A
// malformed or unknown id still returns the pixel — a mail client must never
// see an error — but records nothing.
func (s *Server) handleTrackOpen(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	recipientID := r.URL.Query().Get("s")

	if campaignID != "" && recipientID != "" {
		if tenantID, err := s.tracking.ResolveTenantForCampaign(r.Context(), campaignID); err == nil {
			if err := s.tracking.RecordView(r.Context(), tenantID, campaignID, recipientID); err != nil {
				s.logger.Warn("recording campaign view", "error", err)
			}
		}
	}

	w.Header().Set("Content-Type", "image/gif")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(transparentGIF)
}

// handleTrackClick records a click and redirects to the link's original URL.
// An unknown link id is a 404.
func (s *Server) handleTrackClick(w http.ResponseWriter, r *http.Request) {
	linkID := chi.URLParam(r, "linkId")
	recipientID := r.URL.Query().Get("s")

	tenantID, err := s.tracking.ResolveTenantForLink(r.Context(), linkID)
	if err != nil {
		s.fail(w, "track click", err)
		return
	}
	url, err := s.tracking.RecordClick(r.Context(), tenantID, linkID, recipientID)
	if err != nil {
		s.fail(w, "track click", err)
		return
	}
	http.Redirect(w, r, url, http.StatusFound)
}

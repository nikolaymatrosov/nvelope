package api

import (
	"errors"
	"net/http"
	"strconv"

	campaigncommand "github.com/nikolaymatrosov/nvelope/internal/campaign/app/command"
	campaigndomain "github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// handleTransactionalSend sends one transactional message synchronously. It is
// reached only through the API-key middleware.
func (s *Server) handleTransactionalSend(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())

	var req struct {
		TemplateID      string            `json:"template_id"`
		To              string            `json:"to"`
		SendingDomainID string            `json:"sending_domain_id"`
		FromName        string            `json:"from_name"`
		FromLocalPart   string            `json:"from_local_part"`
		Variables       map[string]string `json:"variables"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}

	res, err := s.campaign.Commands.SendTransactional.Handle(r.Context(), campaigncommand.SendTransactional{
		TenantID: ws.ID, TemplateID: req.TemplateID, To: req.To,
		SendingDomainID: req.SendingDomainID, FromName: req.FromName,
		FromLocalPart: req.FromLocalPart, Variables: req.Variables,
	})
	if err != nil {
		if errors.Is(err, campaigndomain.ErrRateLimited) {
			seconds := int(res.RetryAfter.Seconds())
			if seconds < 1 {
				seconds = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(seconds))
			writeError(w, http.StatusTooManyRequests, "rate-limited",
				"sending is temporarily rate-limited")
			return
		}
		s.fail(w, "transactional send", err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"message_id": res.MessageID})
}

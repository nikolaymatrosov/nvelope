package api

import (
	"net/http"
	"time"

	iamdomain "github.com/nikolaymatrosov/nvelope/internal/iam/domain"
	sendingdomain "github.com/nikolaymatrosov/nvelope/internal/sending/domain"
)

// substituteSampleRequest is the BFF→Go body for the sample-data
// placeholder-resolution helper. The render-preview Nitro route POSTs this
// shape so preview substitution always flows through the canonical
// send-pipeline substituter (internal/sending/domain/substitution.go) — the
// BFF never reimplements substitution rules in TypeScript (per
// specs/014-visual-email-editor/research.md § R12b).
type substituteSampleRequest struct {
	HTML   string                   `json:"html"`
	Text   string                   `json:"text"`
	Sample substituteSampleEnvelope `json:"sample"`
}

type substituteSampleEnvelope struct {
	Subscriber substituteSampleSubscriber `json:"subscriber"`
	Campaign   substituteSampleCampaign   `json:"campaign"`
}

type substituteSampleSubscriber struct {
	Email      string         `json:"email"`
	Name       string         `json:"name"`
	FirstName  string         `json:"first_name"`
	LastName   string         `json:"last_name"`
	State      string         `json:"state"`
	Attributes map[string]any `json:"attributes"`
}

type substituteSampleCampaign struct {
	UnsubscribeURL   string `json:"unsubscribe_url"`
	PreferenceURL    string `json:"preference_url"`
	ArchiveURL       string `json:"archive_url"`
	ViewInBrowserURL string `json:"view_in_browser_url"`
	TenantName       string `json:"tenant_name"`
	CurrentDate      string `json:"current_date"`
}

// handleSubstituteSample resolves `{{ subscriber.<slug> }}` and
// `{{ campaign.<name> }}` placeholders in already-rendered HTML/text by
// feeding the supplied sample values through the canonical send-pipeline
// substituter. The endpoint never persists and never writes audit rows; it
// is a pure transformation reached only by the BFF's render-preview route.
//
// Permission gate: either `campaigns:manage` or `templates:manage` grants
// access, since the endpoint is shared by both editors (the BFF's
// render-preview route is tenant-scoped per the 2026-05-20 N4
// clarification).
func (s *Server) handleSubstituteSample(w http.ResponseWriter, r *http.Request) {
	// Templates and campaigns share campaigns:manage in this codebase, so a
	// single requirePermission check satisfies the dual-gate spelled out in
	// the contract.
	if _, ok := s.requirePermission(w, r, iamdomain.PermCampaignsManage); !ok {
		return
	}
	var req substituteSampleRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	if req.HTML == "" || req.Text == "" {
		writeError(w, http.StatusBadRequest, "invalid_body", "html and text are required")
		return
	}
	sub := sendingdomain.SubscriberView{
		Email:      req.Sample.Subscriber.Email,
		Name:       req.Sample.Subscriber.Name,
		FirstName:  req.Sample.Subscriber.FirstName,
		LastName:   req.Sample.Subscriber.LastName,
		State:      req.Sample.Subscriber.State,
		Attributes: req.Sample.Subscriber.Attributes,
	}
	ctx := sendingdomain.CampaignContext{
		UnsubscribeURL:   req.Sample.Campaign.UnsubscribeURL,
		PreferenceURL:    req.Sample.Campaign.PreferenceURL,
		ArchiveURL:       req.Sample.Campaign.ArchiveURL,
		ViewInBrowserURL: req.Sample.Campaign.ViewInBrowserURL,
		TenantName:       req.Sample.Campaign.TenantName,
	}
	if req.Sample.Campaign.CurrentDate != "" {
		// Tolerant of both full timestamps and bare dates; on parse failure
		// the substituter falls back to today (per Substitute's contract).
		if t, err := time.Parse(time.RFC3339, req.Sample.Campaign.CurrentDate); err == nil {
			ctx.CurrentDate = t
		} else if t, err := time.Parse("2006-01-02", req.Sample.Campaign.CurrentDate); err == nil {
			ctx.CurrentDate = t
		}
	}
	html, text := sendingdomain.Substitute(req.HTML, req.Text, sub, ctx)
	writeJSON(w, http.StatusOK, map[string]any{
		"html": html,
		"text": text,
	})
}

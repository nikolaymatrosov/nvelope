package api

import (
	"net/http"
	"sort"

	audiencequery "github.com/nikolaymatrosov/nvelope/internal/audience/app/query"
	campaigndomain "github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// subscriberMergeTag is one entry in the picker's subscriber section. It is a
// projection of audiencequery.FieldView trimmed to the fields the editor
// actually surfaces.
type subscriberMergeTag struct {
	Slug        string `json:"slug"`
	DisplayName string `json:"displayName"`
	Type        string `json:"type"`
	BuiltIn     bool   `json:"builtIn"`
}

// campaignMergeTag is one entry in the picker's campaign-namespace section.
// The key is what the operator writes inside `{{ campaign.<key> }}`; the
// display name is what the picker labels.
type campaignMergeTag struct {
	Key         string `json:"key"`
	DisplayName string `json:"displayName"`
}

// campaignMergeTagDisplayNames maps the platform's fixed allow-list of
// campaign-namespace keys (defined in
// internal/campaign/domain/visualdoc.go AllowedCampaignMergeTags) to their
// human-readable labels for the merge-tag picker. Adding a new key to the
// allow-list requires adding a label here too — the merge-tag handler returns
// only keys with a matching label so an unlabeled entry stays out of the
// picker until someone names it.
var campaignMergeTagDisplayNames = map[string]string{
	"unsubscribe_url":     "Unsubscribe URL",
	"preference_url":      "Preference URL",
	"archive_url":         "Archive URL",
	"view_in_browser_url": "View in browser URL",
	"tenant_name":         "Tenant name",
	"current_date":        "Current date",
}

// handleListMergeTags returns the editor's merge-tag picker payload — one
// merged list of subscriber fields (built-in + custom) plus the platform's
// campaign-namespace allow-list. Any tenant member can read this; the picker
// is purely display.
func (s *Server) handleListMergeTags(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := principalFromContext(r.Context()); !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "a workspace session is required")
		return
	}
	fields, err := s.audience.Queries.ListFields.Handle(r.Context(),
		audiencequery.ListFields{TenantID: ws.ID})
	if err != nil {
		s.fail(w, "list merge tags", err)
		return
	}
	subscriber := make([]subscriberMergeTag, 0, len(fields))
	for _, f := range fields {
		subscriber = append(subscriber, subscriberMergeTag{
			Slug:        f.Slug,
			DisplayName: f.DisplayName,
			Type:        f.Type,
			BuiltIn:     f.BuiltIn,
		})
	}
	campaign := make([]campaignMergeTag, 0, len(campaigndomain.AllowedCampaignMergeTags))
	for key := range campaigndomain.AllowedCampaignMergeTags {
		label, ok := campaignMergeTagDisplayNames[key]
		if !ok {
			continue
		}
		campaign = append(campaign, campaignMergeTag{Key: key, DisplayName: label})
	}
	// Map iteration order is randomized; sort the campaign list so the
	// response is stable for clients and tests.
	sort.Slice(campaign, func(i, j int) bool { return campaign[i].Key < campaign[j].Key })
	writeJSON(w, http.StatusOK, map[string]any{
		"subscriber": subscriber,
		"campaign":   campaign,
	})
}

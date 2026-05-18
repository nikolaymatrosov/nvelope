package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	campaignquery "github.com/nikolaymatrosov/nvelope/internal/campaign/app/query"
	deliverabilityquery "github.com/nikolaymatrosov/nvelope/internal/deliverability/app/query"
	deliverabilitydomain "github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
	iamdomain "github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

// countsJSON renders the six delivery counts.
func countsJSON(c deliverabilitydomain.Counts) map[string]any {
	return map[string]any{
		"sent": c.Sent, "delivered": c.Delivered, "opened": c.Opened,
		"clicked": c.Clicked, "bounced": c.Bounced, "complained": c.Complained,
	}
}

// handleCampaignAnalytics returns one campaign's analytics view.
func (s *Server) handleCampaignAnalytics(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermCampaignsGet); !ok {
		return
	}
	campaignID := chi.URLParam(r, "id")
	// Verify the campaign exists in the tenant — a missing campaign is a 404,
	// distinct from a campaign that simply has no analytics row yet.
	if _, err := s.campaign.Queries.GetCampaign.Handle(r.Context(),
		campaignquery.GetCampaign{TenantID: ws.ID, CampaignID: campaignID}); err != nil {
		s.fail(w, "campaign analytics", err)
		return
	}
	view, err := s.deliverability.Queries.GetCampaignAnalytics.Handle(r.Context(),
		deliverabilityquery.GetCampaignAnalytics{TenantID: ws.ID, CampaignID: campaignID})
	if err != nil {
		s.fail(w, "campaign analytics", err)
		return
	}
	var refreshedAt any
	if view.HasData {
		refreshedAt = view.RefreshedAt
	}
	c := view.Counts
	writeJSON(w, http.StatusOK, map[string]any{
		"campaignId": campaignID,
		"counts":     countsJSON(c),
		"rates": map[string]any{
			"openRate": c.OpenRate(), "clickRate": c.ClickRate(),
			"bounceRate": c.BounceRate(), "complaintRate": c.ComplaintRate(),
		},
		"refreshedAt": refreshedAt,
	})
}

// handleDashboard returns the workspace deliverability dashboard.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermCampaignsGet); !ok {
		return
	}
	view, err := s.deliverability.Queries.GetDashboard.Handle(r.Context(),
		deliverabilityquery.GetDashboard{TenantID: ws.ID})
	if err != nil {
		s.fail(w, "dashboard", err)
		return
	}
	recent := make([]map[string]any, 0, len(view.Recent))
	for _, rc := range view.Recent {
		recent = append(recent, map[string]any{
			"campaignId":    rc.CampaignID,
			"name":          rc.Name,
			"sent":          rc.Counts.Sent,
			"openRate":      rc.Counts.OpenRate(),
			"bounceRate":    rc.Counts.BounceRate(),
			"complaintRate": rc.Counts.ComplaintRate(),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"totals": countsJSON(view.Totals),
		"deliverability": map[string]any{
			"bounceRate":    view.Totals.BounceRate(),
			"complaintRate": view.Totals.ComplaintRate(),
		},
		"recentCampaigns": recent,
	})
}

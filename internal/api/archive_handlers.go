package api

import (
	"errors"
	"html/template"
	"net/http"

	"github.com/go-chi/chi/v5"

	campaignquery "github.com/nikolaymatrosov/nvelope/internal/campaign/app/query"
	campaigndomain "github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// archiveIndexEntry is one row of the public archive index.
type archiveIndexEntry struct {
	ID   string
	Name string
	Date string
}

// archiveIndexData is the data the archive-index template renders.
type archiveIndexData struct {
	Chrome  publicChrome
	Slug    string
	Entries []archiveIndexEntry
}

// archiveCampaignData is the data the standalone-campaign template renders.
type archiveCampaignData struct {
	Chrome   publicChrome
	Subject  string
	BodyHTML template.HTML
}

// handleArchiveIndex renders the tenant's public archive index.
func (s *Server) handleArchiveIndex(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	entries, err := s.campaign.Queries.ListArchive.Handle(r.Context(),
		campaignquery.ListArchive{TenantID: ws.ID})
	if err != nil {
		s.logger.Error("listing archive", "error", err)
		s.renderPublicError(w, r.Context(), http.StatusInternalServerError,
			"Something went wrong", "Please try again in a moment.")
		return
	}
	data := archiveIndexData{
		Chrome: s.chromeFor(r.Context(), "Archive"),
		Slug:   ws.Slug,
	}
	for _, e := range entries {
		data.Entries = append(data.Entries, archiveIndexEntry{
			ID:   e.ID,
			Name: e.Name,
			Date: e.ArchivedAt.Format("January 2, 2006"),
		})
	}
	s.renderPublic(w, http.StatusOK, "archive_index", data)
}

// handleArchiveCampaign renders one archive-visible campaign as a standalone
// page.
func (s *Server) handleArchiveCampaign(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	view, err := s.campaign.Queries.GetArchivedCampaign.Handle(r.Context(),
		campaignquery.GetArchivedCampaign{TenantID: ws.ID, CampaignID: chi.URLParam(r, "campaignId")})
	if errors.Is(err, campaigndomain.ErrCampaignNotFound) {
		s.renderPublicNotFound(w, r.Context())
		return
	}
	if err != nil {
		s.logger.Error("loading archived campaign", "error", err)
		s.renderPublicError(w, r.Context(), http.StatusInternalServerError,
			"Something went wrong", "Please try again in a moment.")
		return
	}
	s.renderPublic(w, http.StatusOK, "archive_campaign", archiveCampaignData{
		Chrome:   s.chromeFor(r.Context(), view.Subject),
		Subject:  view.Subject,
		BodyHTML: template.HTML(view.BodyHTML),
	})
}

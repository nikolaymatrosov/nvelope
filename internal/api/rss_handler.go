package api

import (
	"encoding/xml"
	"net/http"
	"strings"

	campaignquery "github.com/nikolaymatrosov/nvelope/internal/campaign/app/query"
)

// rssFeed is an RSS 2.0 channel envelope.
type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Items       []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
	Description string `xml:"description"`
}

// handleRSSFeed renders the tenant's archive as an RSS 2.0 feed. A tenant with
// no archive-visible campaigns yields a valid, empty channel.
func (s *Server) handleRSSFeed(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	entries, err := s.campaign.Queries.ListArchive.Handle(r.Context(),
		campaignquery.ListArchive{TenantID: ws.ID})
	if err != nil {
		s.logger.Error("rss list archive", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	base := strings.TrimRight(s.cfg.PublicBaseURL, "/")
	if base == "" {
		base = strings.TrimRight(s.cfg.BaseURL, "/")
	}
	feed := rssFeed{
		Version: "2.0",
		Channel: rssChannel{
			Title:       ws.Name,
			Link:        base + "/t/" + ws.Slug + "/archive",
			Description: "Archive of campaigns from " + ws.Name,
		},
	}
	for _, e := range entries {
		feed.Channel.Items = append(feed.Channel.Items, rssItem{
			Title:       e.Name,
			Link:        base + "/t/" + ws.Slug + "/archive/" + e.ID,
			GUID:        e.ID,
			PubDate:     e.ArchivedAt.UTC().Format("Mon, 02 Jan 2006 15:04:05 MST"),
			Description: e.Subject,
		})
	}
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml.Header))
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(feed); err != nil {
		s.logger.Error("rss encode", "error", err)
	}
}

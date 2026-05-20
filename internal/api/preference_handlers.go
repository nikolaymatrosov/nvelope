package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	audiencecommand "github.com/nikolaymatrosov/nvelope/internal/audience/app/command"
	audiencequery "github.com/nikolaymatrosov/nvelope/internal/audience/app/query"
	tenantquery "github.com/nikolaymatrosov/nvelope/internal/tenant/app/query"
)

// preferenceListItem is one list shown on the preference page.
type preferenceListItem struct {
	ListID     string
	Name       string
	Subscribed bool
}

// preferencesPageData is the data the preference-page template renders.
type preferencesPageData struct {
	Chrome publicChrome
	Token  string
	Email  string
	Name   string
	Lists  []preferenceListItem
	Saved  bool
	Errors []string
}

// unsubscribedPageData is the data the unsubscribe-result template renders.
type unsubscribedPageData struct {
	Chrome  publicChrome
	Heading string
	Message string
}

// resolvePreferenceToken verifies a preference/unsubscribe token and resolves
// the tenant and subscriber it addresses. The token is stateless and signed —
// a tampered or forged token yields ok=false, so no subscriber data is exposed.
func (s *Server) resolvePreferenceToken(r *http.Request) (ws tenantquery.ResolvedWorkspace,
	subscriberID string, ok bool) {

	payload, valid := s.prefSigner.Verify(chi.URLParam(r, "token"))
	if !valid {
		return tenantquery.ResolvedWorkspace{}, "", false
	}
	tenantID, subID, found := strings.Cut(payload, ":")
	if !found || tenantID == "" || subID == "" {
		return tenantquery.ResolvedWorkspace{}, "", false
	}
	ws, err := s.tenant.Queries.LocateWorkspaceByID.Handle(r.Context(),
		tenantquery.LocateWorkspaceByID{TenantID: tenantID})
	if err != nil {
		return tenantquery.ResolvedWorkspace{}, "", false
	}
	return ws, subID, true
}

// handlePreferencesForm renders a subscriber's self-serve preference page.
func (s *Server) handlePreferencesForm(w http.ResponseWriter, r *http.Request) {
	ws, subscriberID, ok := s.resolvePreferenceToken(r)
	if !ok {
		s.renderPublicNotFound(w, r.Context())
		return
	}
	ctx := context.WithValue(r.Context(), tenantCtxKey, ws)
	view, err := s.audience.Queries.GetPreferences.Handle(ctx,
		audiencequery.GetPreferences{TenantID: ws.ID, SubscriberID: subscriberID})
	if err != nil {
		s.renderPublicNotFound(w, ctx)
		return
	}
	s.renderPreferences(w, ctx, chi.URLParam(r, "token"), view, false, nil)
}

// handlePreferencesSubmit applies a subscriber's preference changes.
func (s *Server) handlePreferencesSubmit(w http.ResponseWriter, r *http.Request) {
	ws, subscriberID, ok := s.resolvePreferenceToken(r)
	if !ok {
		s.renderPublicNotFound(w, r.Context())
		return
	}
	ctx := context.WithValue(r.Context(), tenantCtxKey, ws)
	tok := chi.URLParam(r, "token")

	current, err := s.audience.Queries.GetPreferences.Handle(ctx,
		audiencequery.GetPreferences{TenantID: ws.ID, SubscriberID: subscriberID})
	if err != nil {
		s.renderPublicNotFound(w, ctx)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderPreferences(w, ctx, tok, current, false, []string{"The form could not be read."})
		return
	}

	// An unchecked checkbox is absent from the form, so the desired state is
	// derived against the subscriber's known lists, not the posted keys alone.
	lists := make(map[string]bool, len(current.Lists))
	for _, l := range current.Lists {
		lists[l.ListID] = r.PostForm.Has("list:" + l.ListID)
	}
	err = s.audience.Commands.UpdatePreferences.Handle(ctx, audiencecommand.UpdatePreferences{
		TenantID:     ws.ID,
		SubscriberID: subscriberID,
		Name:         strings.TrimSpace(r.PostForm.Get("name")),
		Lists:        lists,
	})
	if err != nil {
		s.renderPreferences(w, ctx, tok, current, false, []string{"Your preferences could not be saved."})
		return
	}
	updated, err := s.audience.Queries.GetPreferences.Handle(ctx,
		audiencequery.GetPreferences{TenantID: ws.ID, SubscriberID: subscriberID})
	if err != nil {
		s.renderPublicNotFound(w, ctx)
		return
	}
	s.renderPreferences(w, ctx, tok, updated, true, nil)
}

// handleUnsubscribe handles both the single-click GET and the RFC 8058
// one-click POST: it unsubscribes the subscriber from every list.
func (s *Server) handleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	ws, subscriberID, ok := s.resolvePreferenceToken(r)
	if !ok {
		s.renderPublicNotFound(w, r.Context())
		return
	}
	ctx := context.WithValue(r.Context(), tenantCtxKey, ws)
	err := s.audience.Commands.PublicUnsubscribe.Handle(ctx, audiencecommand.PublicUnsubscribe{
		TenantID:     ws.ID,
		SubscriberID: subscriberID,
	})
	if err != nil {
		s.logger.Error("public unsubscribe", "error", err)
		s.renderPublicError(w, ctx, http.StatusInternalServerError,
			"Something went wrong", "Please try again in a moment.")
		return
	}
	s.renderPublic(w, http.StatusOK, "unsubscribed", unsubscribedPageData{
		Chrome:  s.chromeFor(ctx, "Unsubscribed"),
		Heading: "You have been unsubscribed",
		Message: "You will no longer receive emails from this sender.",
	})
}

// renderPreferences renders the preference page.
func (s *Server) renderPreferences(w http.ResponseWriter, ctx context.Context, tok string,
	view audiencequery.PreferencesView, saved bool, errs []string) {

	data := preferencesPageData{
		Chrome: s.chromeFor(ctx, "Your preferences"),
		Token:  tok,
		Email:  view.Email,
		Name:   view.Name,
		Saved:  saved,
		Errors: errs,
	}
	for _, l := range view.Lists {
		data.Lists = append(data.Lists, preferenceListItem{
			ListID: l.ListID, Name: l.Name, Subscribed: l.Subscribed,
		})
	}
	s.renderPublic(w, http.StatusOK, "preferences", data)
}

package api

import (
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	audiencecommand "github.com/nikolaymatrosov/nvelope/internal/audience/app/command"
	audiencequery "github.com/nikolaymatrosov/nvelope/internal/audience/app/query"
	audiencedomain "github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

// subscribePageData is the data the subscription-form template renders.
type subscribePageData struct {
	Chrome    publicChrome
	Slug      string
	PageSlug  string
	PageTitle string
	Fields    []audiencedomain.FormField
	Errors    []string
	Submitted bool
	Values    map[string]string
}

// confirmPageData is the data the confirmation-result template renders.
type confirmPageData struct {
	Chrome      publicChrome
	Heading     string
	Message     string
	ShowResend  bool
	ResendToken string
}

// handlePublicSubscribeForm renders a tenant's public subscription form.
func (s *Server) handlePublicSubscribeForm(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	pageSlug := chi.URLParam(r, "pageSlug")

	page, err := s.audience.Queries.GetSubscriptionPage.Handle(r.Context(),
		audiencequery.GetSubscriptionPage{TenantID: ws.ID, Slug: pageSlug})
	if err != nil || !page.Active {
		s.renderPublicNotFound(w, r.Context())
		return
	}
	s.renderSubscribeForm(w, r, page, nil, nil)
}

// handlePublicSubscribeSubmit accepts a public subscription submission.
func (s *Server) handlePublicSubscribeSubmit(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	pageSlug := chi.URLParam(r, "pageSlug")

	page, err := s.audience.Queries.GetSubscriptionPage.Handle(r.Context(),
		audiencequery.GetSubscriptionPage{TenantID: ws.ID, Slug: pageSlug})
	if err != nil || !page.Active {
		s.renderPublicNotFound(w, r.Context())
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderSubscribeForm(w, r, page, []string{"The form could not be read."}, nil)
		return
	}

	values := map[string]string{}
	for key := range r.PostForm {
		values[key] = r.PostForm.Get(key)
	}
	fields := map[string]string{}
	for _, f := range page.Fields {
		fields[f.Key] = r.PostForm.Get(f.Key)
	}

	err = s.audience.Commands.SubmitPublicSubscription.Handle(r.Context(),
		audiencecommand.SubmitPublicSubscription{
			TenantID:   ws.ID,
			TenantSlug: ws.Slug,
			PageSlug:   pageSlug,
			Email:      r.PostForm.Get("email"),
			Fields:     fields,
			SourceKey:  clientIP(r),
		})
	if err == nil {
		s.renderSubscribeSubmitted(w, r, page)
		return
	}

	if errors.Is(err, audiencedomain.ErrSubscriptionPageNotFound) {
		s.renderPublicNotFound(w, r.Context())
		return
	}
	if ae, ok := apperr.As(err); ok && ae.Category() == apperr.IncorrectInput {
		// A throttled submission and a field-validation failure both re-render
		// the form with a message; neither sends a confirmation email.
		s.renderSubscribeForm(w, r, page, []string{ae.Message()}, values)
		return
	}
	s.logger.Error("public subscription submit", "error", err)
	s.renderPublicError(w, r.Context(), http.StatusInternalServerError,
		"Something went wrong", "Please try again in a moment.")
}

// renderSubscribeForm renders the subscription form, optionally with errors and
// previously submitted values.
func (s *Server) renderSubscribeForm(w http.ResponseWriter, r *http.Request,
	page audiencequery.SubscriptionPageView, errs []string, values map[string]string) {

	if values == nil {
		values = map[string]string{}
	}
	status := http.StatusOK
	s.renderPublic(w, status, "subscribe", subscribePageData{
		Chrome:    s.chromeFor(r.Context(), page.Title),
		Slug:      tenantFromContext(r.Context()).Slug,
		PageSlug:  page.Slug,
		PageTitle: page.Title,
		Fields:    page.Fields,
		Errors:    errs,
		Values:    values,
	})
}

// renderSubscribeSubmitted renders the post-submission "check your email" page.
func (s *Server) renderSubscribeSubmitted(w http.ResponseWriter, r *http.Request,
	page audiencequery.SubscriptionPageView) {

	s.renderPublic(w, http.StatusOK, "subscribe", subscribePageData{
		Chrome:    s.chromeFor(r.Context(), page.Title),
		PageTitle: page.Title,
		Submitted: true,
		Values:    map[string]string{},
	})
}

// handleConfirm confirms a pending subscription from a confirmation link.
func (s *Server) handleConfirm(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	tok := chi.URLParam(r, "token")

	_, err := s.audience.Commands.ConfirmSubscription.Handle(r.Context(),
		audiencecommand.ConfirmSubscription{TenantID: ws.ID, Token: tok})
	if err == nil {
		s.renderConfirm(w, r, confirmPageData{
			Heading: "Subscription confirmed",
			Message: "Thank you — your subscription is now active.",
		})
		return
	}
	if errors.Is(err, audiencedomain.ErrConfirmationExpired) {
		s.renderConfirm(w, r, confirmPageData{
			Heading:     "Link expired",
			Message:     "This confirmation link has expired. Request a new one below.",
			ShowResend:  true,
			ResendToken: tok,
		})
		return
	}
	if errors.Is(err, audiencedomain.ErrAddressSuppressed) {
		s.renderConfirm(w, r, confirmPageData{
			Heading: "Subscription not completed",
			Message: "This address cannot be subscribed. Please contact the sender if you believe this is a mistake.",
		})
		return
	}
	s.logger.Error("confirm subscription", "error", err)
	s.renderPublicError(w, r.Context(), http.StatusInternalServerError,
		"Something went wrong", "Please try again in a moment.")
}

// handleResendConfirmation issues a fresh confirmation link for an expired one.
func (s *Server) handleResendConfirmation(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	tok := chi.URLParam(r, "token")

	err := s.audience.Commands.ResendConfirmation.Handle(r.Context(),
		audiencecommand.ResendConfirmation{TenantID: ws.ID, TenantSlug: ws.Slug, Token: tok})
	if errors.Is(err, audiencedomain.ErrPendingSubscriptionNotFound) {
		s.renderPublicNotFound(w, r.Context())
		return
	}
	if err != nil {
		s.logger.Error("resend confirmation", "error", err)
		s.renderPublicError(w, r.Context(), http.StatusInternalServerError,
			"Something went wrong", "Please try again in a moment.")
		return
	}
	s.renderConfirm(w, r, confirmPageData{
		Heading: "Check your email",
		Message: "A fresh confirmation link is on its way to your inbox.",
	})
}

// renderConfirm renders the confirmation-result page with tenant chrome.
func (s *Server) renderConfirm(w http.ResponseWriter, r *http.Request, data confirmPageData) {
	data.Chrome = s.chromeFor(r.Context(), data.Heading)
	s.renderPublic(w, http.StatusOK, "confirm", data)
}

// clientIP extracts the calling client's address for throttling. It trusts the
// left-most X-Forwarded-For entry when present, falling back to the transport
// remote address.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if first := strings.TrimSpace(strings.Split(xff, ",")[0]); first != "" {
			return first
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

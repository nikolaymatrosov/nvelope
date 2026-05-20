package api

import (
	"bytes"
	"context"
	"embed"
	"html/template"
	"net/http"

	tenantquery "github.com/nikolaymatrosov/nvelope/internal/tenant/app/query"
)

// templateFS holds the server-rendered public page templates. They are
// embedded so cmd/api stays stateless and self-contained.
//
//go:embed templates/*.html
var templateFS embed.FS

// publicTpl is the parsed set of public page templates. A parse failure is a
// programming error in an embedded asset, so it panics at startup.
var publicTpl = template.Must(template.ParseFS(templateFS, "templates/*.html"))

// defaultPrimaryColor is the brand colour used on public pages for a tenant
// that has not configured custom branding.
const defaultPrimaryColor = "#4f46e5"

// publicChrome is the per-tenant framing every public page renders inside: the
// page title plus the tenant's branding. Custom CSS is typed template.CSS so
// it is emitted into the page <style> block unescaped — it is sanitised on
// save by the tenant branding domain, never at render time.
type publicChrome struct {
	Title        string
	TenantName   string
	PrimaryColor string
	LogoURL      string
	CustomCSS    template.CSS
}

// defaultChrome is the framing used when no tenant is resolved (an unknown
// slug or token).
func defaultChrome(title string) publicChrome {
	return publicChrome{Title: title, TenantName: "nvelope", PrimaryColor: defaultPrimaryColor}
}

// chromeFor builds the public-page framing for the tenant resolved onto the
// request context, applying that tenant's branding when configured.
func (s *Server) chromeFor(ctx context.Context, title string) publicChrome {
	ws := tenantFromContext(ctx)
	c := publicChrome{Title: title, TenantName: ws.Name, PrimaryColor: defaultPrimaryColor}
	if c.TenantName == "" {
		c.TenantName = "nvelope"
	}
	s.applyBranding(ctx, &c, ws.ID)
	return c
}

// applyBranding overlays a tenant's configured branding onto the page chrome.
// A lookup failure is logged and ignored — public pages then render with the
// platform defaults rather than fail.
func (s *Server) applyBranding(ctx context.Context, c *publicChrome, tenantID string) {
	view, err := s.tenant.Queries.GetBranding.Handle(ctx, tenantquery.GetBranding{TenantID: tenantID})
	if err != nil {
		s.logger.Warn("loading tenant branding", "tenant", tenantID, "error", err)
		return
	}
	if view.PrimaryColor != "" {
		c.PrimaryColor = view.PrimaryColor
	}
	if view.LogoURL != "" {
		c.LogoURL = view.LogoURL
	}
	if view.CustomCSS != "" {
		c.CustomCSS = template.CSS(view.CustomCSS)
	}
}

// errorPage is the data the branded error template renders.
type errorPage struct {
	Chrome  publicChrome
	Heading string
	Message string
}

// renderPublic writes a server-rendered public page. A template execution
// failure is logged and surfaced as a plain 500 — it never leaks a stack
// trace to a public visitor.
func (s *Server) renderPublic(w http.ResponseWriter, status int, name string, data any) {
	var buf bytes.Buffer
	if err := publicTpl.ExecuteTemplate(&buf, name, data); err != nil {
		s.logger.Error("rendering public page", "template", name, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

// renderPublicError renders the branded error page with the chrome of the
// tenant on the request context (default chrome when none is resolved).
func (s *Server) renderPublicError(w http.ResponseWriter, ctx context.Context, status int, heading, message string) {
	chrome := defaultChrome(heading)
	if tenantFromContext(ctx).ID != "" {
		chrome = s.chromeFor(ctx, heading)
	}
	s.renderPublic(w, status, "error", errorPage{Chrome: chrome, Heading: heading, Message: message})
}

// renderPublicNotFound renders the generic branded "not available" page.
func (s *Server) renderPublicNotFound(w http.ResponseWriter, ctx context.Context) {
	s.renderPublicError(w, ctx, http.StatusNotFound, "Not available",
		"This page is not available. The link may be wrong, or the page may have been removed.")
}

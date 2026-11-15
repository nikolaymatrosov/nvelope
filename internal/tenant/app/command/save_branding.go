package command

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

// SaveBranding is the request to configure a tenant's public-page branding.
type SaveBranding struct {
	TenantID     string
	LogoURL      string
	PrimaryColor string
	CustomCSS    string
}

// MediaPublisher makes the media-library assets that content references
// publicly reachable and returns the content with their library URLs
// rewritten to the public copies. Wired in internal/service over the media
// context; text without library URLs passes through unchanged.
type MediaPublisher interface {
	Publish(ctx context.Context, tenantID string, texts ...string) ([]string, error)
}

// SaveBrandingHandler handles the SaveBranding command.
type SaveBrandingHandler struct {
	branding domain.BrandingRepository
	media    MediaPublisher
}

// NewSaveBrandingHandler builds the handler, failing fast on a nil dependency.
func NewSaveBrandingHandler(branding domain.BrandingRepository, media MediaPublisher) SaveBrandingHandler {
	if branding == nil || media == nil {
		panic("nil dependency")
	}
	return SaveBrandingHandler{branding: branding, media: media}
}

// Handle validates and persists the branding. A logo picked from the media
// library is shown on public pages, so it is published now and stored as its
// public URL; an external URL passes through. CSS is sanitised on save so the
// stored value is always safe to render.
func (h SaveBrandingHandler) Handle(ctx context.Context, cmd SaveBranding) error {
	b, err := h.branding.Get(ctx, cmd.TenantID)
	if err != nil {
		return err
	}
	logoURL := cmd.LogoURL
	if logoURL != "" {
		published, err := h.media.Publish(ctx, cmd.TenantID, logoURL)
		if err != nil {
			return err
		}
		logoURL = published[0]
	}
	if err := b.SetLogoURL(logoURL); err != nil {
		return err
	}
	if err := b.SetPrimaryColor(cmd.PrimaryColor); err != nil {
		return err
	}
	if err := b.SetCustomCSS(cmd.CustomCSS); err != nil {
		return err
	}
	return h.branding.Save(ctx, b)
}

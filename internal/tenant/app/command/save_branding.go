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

// SaveBrandingHandler handles the SaveBranding command.
type SaveBrandingHandler struct {
	branding domain.BrandingRepository
}

// NewSaveBrandingHandler builds the handler, failing fast on a nil dependency.
func NewSaveBrandingHandler(branding domain.BrandingRepository) SaveBrandingHandler {
	if branding == nil {
		panic("nil branding repository")
	}
	return SaveBrandingHandler{branding: branding}
}

// Handle validates and persists the branding. CSS is sanitised on save so the
// stored value is always safe to render.
func (h SaveBrandingHandler) Handle(ctx context.Context, cmd SaveBranding) error {
	b, err := h.branding.Get(ctx, cmd.TenantID)
	if err != nil {
		return err
	}
	if err := b.SetLogoURL(cmd.LogoURL); err != nil {
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

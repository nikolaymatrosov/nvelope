package query

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

// BrandingView is the read model for a tenant's public-page branding.
type BrandingView struct {
	LogoURL      string `json:"logo_url"`
	PrimaryColor string `json:"primary_color"`
	CustomCSS    string `json:"custom_css"`
}

// GetBranding is the request for a tenant's branding.
type GetBranding struct {
	TenantID string
}

// GetBrandingHandler handles the GetBranding query.
type GetBrandingHandler struct {
	branding domain.BrandingRepository
}

// NewGetBrandingHandler builds the handler, failing fast on a nil dependency.
func NewGetBrandingHandler(branding domain.BrandingRepository) GetBrandingHandler {
	if branding == nil {
		panic("nil branding repository")
	}
	return GetBrandingHandler{branding: branding}
}

// Handle returns the tenant's branding, or platform defaults when unset.
func (h GetBrandingHandler) Handle(ctx context.Context, q GetBranding) (BrandingView, error) {
	b, err := h.branding.Get(ctx, q.TenantID)
	if err != nil {
		return BrandingView{}, err
	}
	return BrandingView{
		LogoURL:      b.LogoURL(),
		PrimaryColor: b.PrimaryColor(),
		CustomCSS:    b.CustomCSS(),
	}, nil
}

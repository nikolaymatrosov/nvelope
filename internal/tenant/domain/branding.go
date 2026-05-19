package domain

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

// colorPattern matches a 3- or 6-digit hex CSS colour, with a leading '#'.
var colorPattern = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// maxCustomCSSBytes bounds a tenant's stored custom CSS so a single oversized
// payload cannot dominate a public page.
const maxCustomCSSBytes = 64 * 1024

// unsafeCSSSubstrings is the deny-list applied to tenant custom CSS on save.
// Modern browsers do not execute script from CSS, so the residual risks the
// platform must block are markup-break-out, remote imports, and exfiltration
// via url(): containing those plus emitting the CSS inside a scoped wrapper
// satisfies FR-022. The check is case-insensitive.
var unsafeCSSSubstrings = []string{
	"</style",
	"@import",
	"expression(",
	"javascript:",
	"behavior:",
}

// TenantBranding is per-tenant configuration applied to every public page of
// the tenant: logo, primary colour, and a sanitised block of custom CSS. It is
// a tenant-plane aggregate; the row is keyed on tenant_id (one branding per
// tenant) and reached only through the RLS-bound transaction owned by its
// repository adapter.
type TenantBranding struct {
	tenantID     string
	logoURL      string
	primaryColor string
	customCSS    string
	updatedAt    time.Time
}

// NewTenantBranding builds an empty (platform-default) branding for a tenant.
func NewTenantBranding(tenantID string) *TenantBranding {
	return &TenantBranding{tenantID: tenantID}
}

// HydrateTenantBranding reconstructs a branding row from the database.
// Persistence only — it performs no validation.
func HydrateTenantBranding(tenantID, logoURL, primaryColor, customCSS string, updatedAt time.Time) *TenantBranding {
	return &TenantBranding{
		tenantID: tenantID, logoURL: logoURL, primaryColor: primaryColor,
		customCSS: customCSS, updatedAt: updatedAt,
	}
}

// TenantID returns the owning tenant's id.
func (b *TenantBranding) TenantID() string { return b.tenantID }

// LogoURL returns the configured logo URL, or "".
func (b *TenantBranding) LogoURL() string { return b.logoURL }

// PrimaryColor returns the configured primary hex colour, or "".
func (b *TenantBranding) PrimaryColor() string { return b.primaryColor }

// CustomCSS returns the sanitised custom CSS, or "".
func (b *TenantBranding) CustomCSS() string { return b.customCSS }

// UpdatedAt returns when the branding was last changed.
func (b *TenantBranding) UpdatedAt() time.Time { return b.updatedAt }

// SetLogoURL configures the logo URL. An empty value clears it; a non-empty
// value must look like an https:// URL.
func (b *TenantBranding) SetLogoURL(url string) error {
	url = strings.TrimSpace(url)
	if url == "" {
		b.logoURL = ""
		return nil
	}
	lower := strings.ToLower(url)
	if !strings.HasPrefix(lower, "https://") && !strings.HasPrefix(lower, "http://") {
		return apperr.NewIncorrectInput("invalid_logo_url",
			"the logo URL must start with http(s)://")
	}
	b.logoURL = url
	return nil
}

// SetPrimaryColor configures the primary colour. An empty value clears it.
func (b *TenantBranding) SetPrimaryColor(hex string) error {
	hex = strings.TrimSpace(hex)
	if hex == "" {
		b.primaryColor = ""
		return nil
	}
	if !colorPattern.MatchString(hex) {
		return ErrInvalidBrandingColor
	}
	b.primaryColor = hex
	return nil
}

// SetCustomCSS configures the tenant's custom CSS, sanitising it on save: a
// stored value is always safe to render, and the check runs once. Constructs
// that could break out of the public-page <style> block, fetch remote content,
// or execute script are rejected.
func (b *TenantBranding) SetCustomCSS(css string) error {
	if len(css) > maxCustomCSSBytes {
		return ErrUnsafeCSS.WithMessage("custom CSS is too large")
	}
	lower := strings.ToLower(css)
	for _, bad := range unsafeCSSSubstrings {
		if strings.Contains(lower, bad) {
			return ErrUnsafeCSS
		}
	}
	if strings.Contains(lower, "url(") && containsNonHTTPSURL(lower) {
		return ErrUnsafeCSS.WithMessage("custom CSS may reference only https:// urls")
	}
	b.customCSS = css
	return nil
}

// containsNonHTTPSURL reports whether any url(...) reference is not https://.
func containsNonHTTPSURL(lower string) bool {
	rest := lower
	for {
		i := strings.Index(rest, "url(")
		if i < 0 {
			return false
		}
		j := strings.IndexByte(rest[i+4:], ')')
		if j < 0 {
			return true
		}
		inner := strings.TrimSpace(rest[i+4 : i+4+j])
		inner = strings.Trim(inner, "'\"")
		if !strings.HasPrefix(inner, "https://") {
			return true
		}
		rest = rest[i+4+j+1:]
	}
}

// BrandingRepository persists tenant branding. Every operation runs inside a
// tenant-bound transaction.
type BrandingRepository interface {
	// Get returns the tenant's branding, or platform defaults (an empty value)
	// when no row exists.
	Get(ctx context.Context, tenantID string) (*TenantBranding, error)
	// Save upserts the tenant's branding row.
	Save(ctx context.Context, b *TenantBranding) error
}

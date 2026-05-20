package domain

import (
	"regexp"
	"strings"

	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

// Theme is the colors, fonts, and container width used as defaults inside
// the visual editor and the server-side renderer. A row's `theme` column is
// either NULL (inherit tenant Phase 6 branding at render time, per FR-022/
// FR-024) or carries an explicit override the operator pinned (per FR-023).
//
// Theme is a value object: it has no identity and is constructed through
// NewTheme. HydrateTheme is the persistence-only path.
type Theme struct {
	textColor       string
	linkColor       string
	buttonColor     string
	buttonTextColor string
	fontFamily      string
	containerWidth  int
}

// cssColorRE recognizes the three CSS color shapes the editor and renderer
// support: hex (#rgb, #rrggbb, #rrggbbaa), rgb()/rgba(), and a small set of
// CSS named colors. The renderer relies on the value going untouched into
// inline-style attributes, so anything beyond these shapes is refused.
var cssColorRE = regexp.MustCompile(
	`^(#[0-9a-fA-F]{3,8}|rgba?\([\s\d\.,%/]+\)|[a-zA-Z]{3,30})$`,
)

// NewTheme builds a validated Theme override.
func NewTheme(textColor, linkColor, buttonColor, buttonTextColor, fontFamily string, containerWidth int) (*Theme, error) {
	for _, c := range []string{textColor, linkColor, buttonColor, buttonTextColor} {
		if !cssColorRE.MatchString(strings.TrimSpace(c)) {
			return nil, ErrThemeInvalidColor
		}
	}
	if strings.TrimSpace(fontFamily) == "" || len(fontFamily) > 256 {
		return nil, ErrThemeInvalidFont
	}
	if containerWidth < 320 || containerWidth > 800 {
		return nil, ErrThemeInvalidWidth
	}
	return &Theme{
		textColor:       textColor,
		linkColor:       linkColor,
		buttonColor:     buttonColor,
		buttonTextColor: buttonTextColor,
		fontFamily:      fontFamily,
		containerWidth:  containerWidth,
	}, nil
}

// HydrateTheme reconstructs a Theme from persisted JSON without validation.
func HydrateTheme(textColor, linkColor, buttonColor, buttonTextColor, fontFamily string, containerWidth int) *Theme {
	return &Theme{
		textColor:       textColor,
		linkColor:       linkColor,
		buttonColor:     buttonColor,
		buttonTextColor: buttonTextColor,
		fontFamily:      fontFamily,
		containerWidth:  containerWidth,
	}
}

// TextColor returns the body-copy text color.
func (t *Theme) TextColor() string { return t.textColor }

// LinkColor returns the color applied to inline links.
func (t *Theme) LinkColor() string { return t.linkColor }

// ButtonColor returns the background color applied to button blocks.
func (t *Theme) ButtonColor() string { return t.buttonColor }

// ButtonTextColor returns the foreground color applied to button labels.
func (t *Theme) ButtonTextColor() string { return t.buttonTextColor }

// FontFamily returns the CSS font-family value.
func (t *Theme) FontFamily() string { return t.fontFamily }

// ContainerWidth returns the pixel width of the email container.
func (t *Theme) ContainerWidth() int { return t.containerWidth }

// DefaultsFromBranding builds a Theme value from a tenant's Phase 6
// branding primitives. Used by the renderer when the row's theme column is
// NULL — the inheritance behavior FR-022 and FR-024 require.
//
// brandingPrimaryColor is the tenant's branding primary color (e.g.
// "#0066cc"). The other defaults follow platform conventions; future Phase
// 6 work may extend the branding schema to include them, at which point
// this function gains parameters.
func DefaultsFromBranding(brandingPrimaryColor string) Theme {
	primary := strings.TrimSpace(brandingPrimaryColor)
	if !cssColorRE.MatchString(primary) {
		primary = defaultThemePrimary
	}
	return Theme{
		textColor:       "#222222",
		linkColor:       primary,
		buttonColor:     primary,
		buttonTextColor: "#ffffff",
		fontFamily:      "'Inter', -apple-system, BlinkMacSystemFont, sans-serif",
		containerWidth:  600,
	}
}

const defaultThemePrimary = "#0066cc"

// Sanity-check the platform default at package load: it must satisfy the
// same shape the validator enforces for operator-supplied colors.
func init() {
	if !cssColorRE.MatchString(defaultThemePrimary) {
		panic("invalid platform default primary color")
	}
}

// ── Typed errors ────────────────────────────────────────────────────────

var (
	// ErrThemeInvalidColor is returned when any of the four theme colors
	// fails the CSS-color shape check.
	ErrThemeInvalidColor = apperr.NewIncorrectInput("theme_invalid_color",
		"theme color must be a CSS color value")

	// ErrThemeInvalidFont is returned when the font-family value is empty
	// or longer than 256 characters.
	ErrThemeInvalidFont = apperr.NewIncorrectInput("theme_invalid_font",
		"theme font-family must be between 1 and 256 characters")

	// ErrThemeInvalidWidth is returned when the container width falls
	// outside the [320, 800] pixel range.
	ErrThemeInvalidWidth = apperr.NewIncorrectInput("theme_invalid_width",
		"theme container width must be between 320 and 800 pixels")
)

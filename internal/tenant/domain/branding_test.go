package domain_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

func TestBrandingSetPrimaryColor(t *testing.T) {
	t.Parallel()
	b := domain.NewTenantBranding("t-1")
	require.NoError(t, b.SetPrimaryColor("#4f46e5"))
	require.Equal(t, "#4f46e5", b.PrimaryColor())
	require.NoError(t, b.SetPrimaryColor(""))
	require.Equal(t, "", b.PrimaryColor())
	require.ErrorIs(t, b.SetPrimaryColor("hotpink"), domain.ErrInvalidBrandingColor)
	require.ErrorIs(t, b.SetPrimaryColor("#zzz"), domain.ErrInvalidBrandingColor)
}

func TestBrandingSetLogoURL(t *testing.T) {
	t.Parallel()
	b := domain.NewTenantBranding("t-1")
	require.NoError(t, b.SetLogoURL("https://cdn.example.com/logo.png"))
	require.Equal(t, "https://cdn.example.com/logo.png", b.LogoURL())
	require.NoError(t, b.SetLogoURL(""))
	require.Equal(t, "", b.LogoURL())
	require.Error(t, b.SetLogoURL("javascript:alert(1)"))
	require.Error(t, b.SetLogoURL("/local/path"))
}

func TestBrandingCustomCSSAcceptsBenign(t *testing.T) {
	t.Parallel()
	b := domain.NewTenantBranding("t-1")
	css := `.nv-public { background: #fafafa; }
            .nv-public a { color: #4f46e5; }
            .nv-public img { max-width: 100%; }`
	require.NoError(t, b.SetCustomCSS(css))
	require.Equal(t, css, b.CustomCSS())
}

func TestBrandingCustomCSSRejectsUnsafeConstructs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		css  string
	}{
		{"style-breakout", "body { color: red; } </style><script>x()</script>"},
		{"at-import", "@import url('https://evil.example/css');"},
		{"expression", ".x { width: expression(alert(1)); }"},
		{"javascript-url", "body { background: url('javascript:alert(1)'); }"},
		{"behavior", ".x { behavior: url(evil.htc); }"},
		{"non-https-url", "body { background: url(http://insecure.example/i.png); }"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := domain.NewTenantBranding("t-1")
			require.ErrorIs(t, b.SetCustomCSS(c.css), domain.ErrUnsafeCSS)
		})
	}
}

func TestBrandingCustomCSSRejectsOversized(t *testing.T) {
	t.Parallel()
	b := domain.NewTenantBranding("t-1")
	huge := strings.Repeat("a", 65*1024)
	require.ErrorIs(t, b.SetCustomCSS(huge), domain.ErrUnsafeCSS)
}

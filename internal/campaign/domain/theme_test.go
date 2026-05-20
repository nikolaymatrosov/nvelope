package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

func TestNewTheme_Valid(t *testing.T) {
	t.Parallel()
	th, err := domain.NewTheme("#222222", "#0066cc", "#0066cc", "#ffffff",
		"'Inter', sans-serif", 600)
	require.NoError(t, err)
	require.Equal(t, "#222222", th.TextColor())
	require.Equal(t, "#0066cc", th.LinkColor())
	require.Equal(t, "#0066cc", th.ButtonColor())
	require.Equal(t, "#ffffff", th.ButtonTextColor())
	require.Equal(t, "'Inter', sans-serif", th.FontFamily())
	require.Equal(t, 600, th.ContainerWidth())
}

func TestNewTheme_AcceptsCssColorShapes(t *testing.T) {
	t.Parallel()
	for _, c := range []string{"#abc", "#aabbcc", "#aabbccdd", "rgb(0,0,0)", "rgba(0,0,0,0.5)", "blue", "cornflowerblue"} {
		_, err := domain.NewTheme(c, c, c, c, "Inter", 600)
		require.NoError(t, err, "color=%q", c)
	}
}

func TestNewTheme_RejectsBadColor(t *testing.T) {
	t.Parallel()
	for _, c := range []string{"", "rgba(", "javascript:0", "#xyzxyz", "  "} {
		_, err := domain.NewTheme(c, "#000", "#000", "#000", "Inter", 600)
		require.ErrorIs(t, err, domain.ErrThemeInvalidColor, "color=%q", c)
	}
}

func TestNewTheme_RejectsBadFont(t *testing.T) {
	t.Parallel()
	_, err := domain.NewTheme("#000", "#000", "#000", "#000", "", 600)
	require.ErrorIs(t, err, domain.ErrThemeInvalidFont)
	long := make([]byte, 257)
	for i := range long {
		long[i] = 'a'
	}
	_, err = domain.NewTheme("#000", "#000", "#000", "#000", string(long), 600)
	require.ErrorIs(t, err, domain.ErrThemeInvalidFont)
}

func TestNewTheme_RejectsBadWidth(t *testing.T) {
	t.Parallel()
	for _, w := range []int{0, 319, 801, 9999} {
		_, err := domain.NewTheme("#000", "#000", "#000", "#000", "Inter", w)
		require.ErrorIs(t, err, domain.ErrThemeInvalidWidth, "w=%d", w)
	}
}

func TestDefaultsFromBranding_ValidPrimary(t *testing.T) {
	t.Parallel()
	th := domain.DefaultsFromBranding("#cc3366")
	require.Equal(t, "#cc3366", th.LinkColor())
	require.Equal(t, "#cc3366", th.ButtonColor())
	require.Equal(t, "#222222", th.TextColor())
	require.Equal(t, "#ffffff", th.ButtonTextColor())
	require.Equal(t, 600, th.ContainerWidth())
	require.NotEmpty(t, th.FontFamily())
}

func TestDefaultsFromBranding_FallsBackToPlatformDefault(t *testing.T) {
	t.Parallel()
	// Garbage / empty branding primary falls back to the platform default
	// rather than producing an invalid Theme.
	th := domain.DefaultsFromBranding("javascript:alert(1)")
	require.Equal(t, "#0066cc", th.LinkColor())
	require.Equal(t, "#0066cc", th.ButtonColor())

	th2 := domain.DefaultsFromBranding("")
	require.Equal(t, "#0066cc", th2.LinkColor())
}

func TestHydrateTheme_RoundTrip(t *testing.T) {
	t.Parallel()
	th := domain.HydrateTheme("#111111", "#222222", "#333333", "#ffffff", "Inter", 640)
	require.Equal(t, "#111111", th.TextColor())
	require.Equal(t, 640, th.ContainerWidth())
}

package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
)

func TestNewLocaleAcceptsSupportedCodes(t *testing.T) {
	t.Parallel()

	for _, code := range []string{"en", "ru", "EN", " ru "} {
		l, err := domain.NewLocale(code)
		require.NoError(t, err, "code %q is supported", code)
		require.False(t, l.IsZero())
	}
}

func TestNewLocaleRejectsUnsupportedCodes(t *testing.T) {
	t.Parallel()

	for _, code := range []string{"", "de", "xx", "english", "e n"} {
		_, err := domain.NewLocale(code)
		require.Error(t, err, "code %q is not supported", code)
	}
}

func TestNewLocaleNormalizesCase(t *testing.T) {
	t.Parallel()

	l, err := domain.NewLocale("  RU ")
	require.NoError(t, err)
	require.Equal(t, "ru", l.String())
}

func TestHydrateLocaleUnknownValueIsUnset(t *testing.T) {
	t.Parallel()

	// An empty or unrecognised stored value degrades to the unset Locale
	// rather than erroring (FR-014).
	require.True(t, domain.HydrateLocale("").IsZero())
	require.True(t, domain.HydrateLocale("de").IsZero())
	require.Equal(t, "ru", domain.HydrateLocale("ru").String())
}

func TestZeroLocaleIsUnset(t *testing.T) {
	t.Parallel()

	var l domain.Locale
	require.True(t, l.IsZero())
	require.Empty(t, l.String())
}

func TestDefaultLocaleIsEnglish(t *testing.T) {
	t.Parallel()

	require.Equal(t, "en", domain.DefaultLocale.String())
	require.False(t, domain.DefaultLocale.IsZero())
}

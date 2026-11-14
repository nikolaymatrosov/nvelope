package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/media/domain"
)

const (
	testBase = "https://app.test"
	assetA   = "0b9f3c3e-6a7d-4c1e-9a55-2f1d7e4b8c90"
	assetB   = "1c2d3e4f-0000-4000-8000-000000000001"
)

func TestLibraryURL(t *testing.T) {
	require.Equal(t, "https://app.test/t/acme/api/media/"+assetA+"/content",
		domain.LibraryURL("https://app.test/", "acme", assetA))
}

func TestLibraryURLs_Is(t *testing.T) {
	urls := domain.NewLibraryURLs(testBase)
	require.True(t, urls.Is(domain.LibraryURL(testBase, "acme", assetA)))
	for _, ref := range []string{
		"https://evil.test/t/acme/api/media/" + assetA + "/content",
		"https://app.test/t/acme/api/media/not-a-uuid/content",
		"https://app.test/t/acme/api/media/" + assetA + "/content?x=1",
		"https://nvelope-media.storage.yandexcloud.net/media/t1/" + assetA + "/a.png",
		"",
	} {
		require.False(t, urls.Is(ref), ref)
	}
}

func TestLibraryURLs_AssetIDsAndReplace(t *testing.T) {
	urls := domain.NewLibraryURLs(testBase)
	html := `<img src="` + domain.LibraryURL(testBase, "acme", assetA) + `">` +
		`<a href="` + domain.LibraryURL(testBase, "acme", assetB) + `">pdf</a>` +
		`<td style="background:url(` + domain.LibraryURL(testBase, "acme", assetA) + `)">`
	text := "Download: " + domain.LibraryURL(testBase, "acme", assetB)

	require.Equal(t, []string{assetA, assetB}, urls.AssetIDs(html, text))
	require.Empty(t, urls.AssetIDs("<p>no media</p>"))

	got := urls.Replace(html, func(id string) string { return "https://pub.test/" + id })
	require.Equal(t, `<img src="https://pub.test/`+assetA+`">`+
		`<a href="https://pub.test/`+assetB+`">pdf</a>`+
		`<td style="background:url(https://pub.test/`+assetA+`)">`, got)
}

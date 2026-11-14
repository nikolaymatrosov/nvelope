package api

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

// testPNG encodes a transparent w×h PNG.
func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, w, h))))
	return buf.Bytes()
}

// uploadMedia posts one file to the media library as the signed-in user.
func (ts *testServer) uploadMedia(slug, filename string, data []byte) (int, map[string]any) {
	ts.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", filename)
	require.NoError(ts.t, err)
	_, err = part.Write(data)
	require.NoError(ts.t, err)
	require.NoError(ts.t, mw.Close())

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/t/"+slug+"/api/media", &buf)
	require.NoError(ts.t, err)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := ts.client.Do(req)
	require.NoError(ts.t, err)
	defer func() { _ = resp.Body.Close() }()
	var decoded map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&decoded)
	return resp.StatusCode, decoded
}

// noRedirects returns a client sharing c's cookies that reports redirects
// instead of following them.
func noRedirects(c *http.Client) *http.Client {
	return &http.Client{
		Transport: c.Transport, Jar: c.Jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func TestMediaLibraryIsPrivate(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)

	status, body := ts.uploadMedia(slug, "logo.png", testPNG(t, 10, 10))
	require.Equal(t, http.StatusCreated, status)
	id, _ := body["id"].(string)
	libraryURL := "https://app.test/t/" + slug + "/api/media/" + id + "/content"
	require.Equal(t, libraryURL, body["url"])
	require.EqualValues(t, 10, body["width"])

	status, body = ts.request(http.MethodGet, "/t/"+slug+"/api/media", nil)
	require.Equal(t, http.StatusOK, status)
	items, _ := body["items"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	require.Equal(t, libraryURL, item["url"])
	require.NotEmpty(t, item["preview_url"])
	require.Equal(t, false, item["published"])
	require.NotContains(t, item, "public_url")

	contentPath := ts.URL + "/t/" + slug + "/api/media/" + id + "/content"
	resp, err := noRedirects(ts.client).Get(contentPath)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusFound, resp.StatusCode)
	require.True(t, strings.HasPrefix(resp.Header.Get("Location"), "https://media.test/media/"),
		resp.Header.Get("Location"))
	require.Equal(t, "private, max-age=600", resp.Header.Get("Cache-Control"))

	// Without a session there is no redirect to the bytes.
	resp, err = noRedirects(ts.newClient()).Get(contentPath)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.NotEqual(t, http.StatusFound, resp.StatusCode)
}

func TestMediaUploadRejectsSVG(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)

	status, body := ts.uploadMedia(slug, "logo.svg",
		[]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`))
	require.Equal(t, http.StatusUnprocessableEntity, status)
	require.Equal(t, "unsupported_media_type", body["code"])
}

// startCampaignWithLogo signs up, uploads a PNG, creates a campaign whose
// raw HTML embeds the PNG's library URL, and starts it. It returns the API
// base path, the asset id, and the campaign id.
func (ts *testServer) startCampaignWithLogo() (base, assetID, campaignID string) {
	t := ts.t
	t.Helper()
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base = "/t/" + slug + "/api"
	domainID := ts.seedVerifiedDomain(slug, "media-"+dbtest.RandString()+".test")
	listID := ts.seedSubscribersOnList(slug, []string{"a@example.com"})

	status, up := ts.uploadMedia(slug, "logo.png", testPNG(t, 10, 10))
	require.Equal(t, http.StatusCreated, status)
	assetID, _ = up["id"].(string)
	libraryURL, _ := up["url"].(string)

	status, body := ts.request(http.MethodPost, base+"/campaigns", map[string]any{
		"name": "With logo", "subject": "News",
		"body_html":         `<p><img src="` + libraryURL + `" alt="logo"></p>`,
		"body_text":         "News",
		"from_name":         "Acme",
		"from_local_part":   "news",
		"sending_domain_id": domainID,
		"list_ids":          []string{listID},
	})
	require.Equal(t, http.StatusCreated, status)
	campaignID, _ = body["id"].(string)

	status, _ = ts.request(http.MethodPost, base+"/campaigns/"+campaignID+"/start", nil)
	require.Equal(t, http.StatusAccepted, status)
	return base, assetID, campaignID
}

func TestCampaignStartPublishesLibraryMedia(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	base, assetID, campaignID := ts.startCampaignWithLogo()

	_, body := ts.request(http.MethodGet, base+"/campaigns/"+campaignID, nil)
	html, _ := body["body_html"].(string)
	require.NotContains(t, html, "/api/media/"+assetID+"/content")
	require.Contains(t, html, "https://media.test/published/")

	_, body = ts.request(http.MethodGet, base+"/media", nil)
	item := body["items"].([]any)[0].(map[string]any)
	require.Equal(t, true, item["published"])
}

func TestCampaignStartBlockedByDeletedMedia(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"
	domainID := ts.seedVerifiedDomain(slug, "media-"+dbtest.RandString()+".test")
	listID := ts.seedSubscribersOnList(slug, []string{"a@example.com"})

	_, up := ts.uploadMedia(slug, "logo.png", testPNG(t, 10, 10))
	assetID, _ := up["id"].(string)
	libraryURL, _ := up["url"].(string)
	status, body := ts.request(http.MethodPost, base+"/campaigns", map[string]any{
		"name": "Broken", "subject": "News",
		"body_html":         `<p><img src="` + libraryURL + `"></p>`,
		"from_name":         "Acme",
		"from_local_part":   "news",
		"sending_domain_id": domainID,
		"list_ids":          []string{listID},
	})
	require.Equal(t, http.StatusCreated, status)
	campaignID, _ := body["id"].(string)

	status, _ = ts.request(http.MethodDelete, base+"/media/"+assetID, nil)
	require.Equal(t, http.StatusNoContent, status)

	status, body = ts.request(http.MethodPost, base+"/campaigns/"+campaignID+"/start", nil)
	require.Equal(t, http.StatusUnprocessableEntity, status)
	require.Equal(t, "media_missing", body["code"])

	_, body = ts.request(http.MethodGet, base+"/campaigns/"+campaignID, nil)
	require.Equal(t, "draft", body["status"])
}

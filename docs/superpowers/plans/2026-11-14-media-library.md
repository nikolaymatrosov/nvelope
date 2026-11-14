# Private Media Library Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the tenant media library private, publish a public copy of an asset only when content that uses it goes live, and normalize uploads server-side (type sniffing, EXIF stripping, downscaling, WebP conversion).

**Architecture:** Content stores a *library URL* on the app origin (`{BASE_URL}/t/{slug}/api/media/{id}/content`). For a member, Go answers it with a 302 to a presigned URL. Publishing (campaign start, transactional template save, branding save) copies the private object to `published/…` with `public-read` and rewrites library URLs to public URLs. The campaign and tenant contexts reach the media context through a consumer-owned `MediaPublisher` port, bridged in `internal/service`.

**Tech Stack:** Go 1.26, pgx, aws-sdk-go-v2 (S3), `golang.org/x/image` (new), TanStack Start + Nitro BFF, react-email, TipTap, vitest.

**Spec:** [docs/superpowers/specs/2026-11-14-media-library-design.md](../specs/2026-11-14-media-library-design.md)

## Global Constraints

- **Go dependency.** The only new Go dependency is `golang.org/x/image v0.48.0`. No new npm dependencies.
- **Library URL format.** `{cfg.BaseURL}/t/{slug}/api/media/{assetId}/content`. The asset id is a lowercase UUID.
- **Object keys.**
  - Library objects: `media/{tenantId}/{assetId}/{filename}`, private (no ACL).
  - Public copies: `published/{tenantId}/{assetId}/{filename}`, ACL `public-read`.
- **Image limits.**
  - Images wider than 1200 px are downscaled to 1200 px.
  - A header that declares more than 25 megapixels is rejected before decode.
  - JPEG quality is 85.
  - At most 2 concurrent decodes per process.
- **Presign TTLs.**
  - Content redirect: 15 min, sent with `Cache-Control: private, max-age=600`.
  - List `preview_url`: 1 h.
  - SPA media-list `staleTime`: 30 min.
- **Error codes** (422 unless noted):
  - `unsupported_media_type`
  - `image_too_large`
  - `media_missing`
  - `invalid_media_ref` (unchanged)
- **Image block `width`.** Optional integer from 1 to 600. The editor sets `min(naturalWidth, 600)`.
- **SQL placeholders.** pgx adapters use `pgx.NamedArgs` for 4 or more placeholders, positional `$N` below that (CLAUDE.md).
- **Paths.** Use paths relative to the repo root. Never prefix commands with `cd <repo root> &&`.
- **Go integration tests need Docker.** Run them as:
  `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock"; go test ./...`
- **Frontend commands need Node on PATH.**
  - Prefix pnpm/node commands with `export PATH="$HOME/.nvm/versions/node/v24.19.0/bin:$PATH";`.
  - Run frontend tests with `.claude/scripts/test-frontend.sh <path>`.
- **Architecture lint.** `make lint-arch` needs `export PATH="$HOME/go/bin:$PATH"`.
- **Commits.** Use conventional commit messages (`feat(media): …`). Never add a `Co-Authored-By` trailer.
- **Flaky tests.**
  - `TestPublicSubscriptionCrossTenantIsolation` (`hash-a`) is a known flaky test.
  - "no migration found for version N" and "unexpected postmaster exit" mean the shared `nvelope-test-pg` container collided with another run.
  - In both cases, re-run. Do not remove the container.

---

### Task 1: Image pipeline

**Files:**
- Create: `internal/media/domain/imaging.go`
- Create: `internal/media/domain/imaging_test.go`
- Modify: `internal/media/domain/errors.go`
- Modify: `go.mod`, `go.sum`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `domain.ProcessUpload(data []byte) (domain.Processed, error)`.
  - `type domain.Processed struct { Data []byte; ContentType string; Width, Height int }`. `Width` and `Height` are 0 for PDF.
  - `domain.FilenameFor(filename, contentType string) string`.
  - `domain.MaxImageWidth = 1200`.
  - `domain.ErrImageTooLarge`, with slug `image_too_large`.
  - Rejections return the existing `domain.ErrUnsupportedMediaType`.

- [ ] **Step 1: Add the dependency**

Run: `go get golang.org/x/image@v0.48.0`
Expected: `go.mod` gains `golang.org/x/image v0.48.0`. It stays marked `// indirect` until Step 4 imports it; `go mod tidy` in Step 5 fixes that.

- [ ] **Step 2: Write the failing tests**

Create `internal/media/domain/imaging_test.go`:

```go
package domain_test

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/media/domain"
)

// 1×1 WebP samples: a lossy (opaque) VP8 and a VP8X with an alpha chunk. Go
// has no WebP encoder, so the fixtures are literal bytes.
const (
	webpLossy1x1 = "UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA"
	webpAlpha1x1 = "UklGRkoAAABXRUJQVlA4WAoAAAAQAAAAAAAAAAAAQUxQSAwAAAARBxAR/Q9ERP8DAABWUDggGAAAABQBAJ0BKgEAAQAAAP4AAA3AAP7mtQAAAA=="
)

// halves is a w×h image whose left half is red and right half is blue, so
// a test can tell which way it was rotated.
func halves(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.NRGBA{R: 255, A: 255}
			if x >= w/2 {
				c = color.NRGBA{B: 255, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func encodeJPEG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	require.NoError(t, jpeg.Encode(&b, img, &jpeg.Options{Quality: 95}))
	return b.Bytes()
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	require.NoError(t, png.Encode(&b, img))
	return b.Bytes()
}

// withEXIF splices an APP1 Exif segment carrying the given orientation and a
// GPS IFD pointer right after the JPEG's SOI marker.
func withEXIF(jpg []byte, orientation uint16) []byte {
	tiff := []byte{'M', 'M', 0, 42, 0, 0, 0, 8, // big-endian TIFF header, IFD0 at 8
		0, 2, // two entries
		0x01, 0x12, 0, 3, 0, 0, 0, 1, byte(orientation >> 8), byte(orientation), 0, 0, // Orientation SHORT
		0x88, 0x25, 0, 4, 0, 0, 0, 1, 0, 0, 0, 0, // GPSInfo IFD pointer LONG
		0, 0, 0, 0} // no next IFD
	payload := append([]byte("Exif\x00\x00"), tiff...)
	n := len(payload) + 2
	out := append([]byte{}, jpg[:2]...)
	out = append(out, 0xFF, 0xE1, byte(n>>8), byte(n))
	out = append(out, payload...)
	return append(out, jpg[2:]...)
}

// pngHeader is a PNG signature plus a valid IHDR declaring w×h and nothing
// else — enough for DecodeConfig, with no pixel data behind it.
func pngHeader(w, h uint32) []byte {
	var b bytes.Buffer
	b.WriteString("\x89PNG\r\n\x1a\n")
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8], ihdr[9] = 8, 2 // 8-bit RGB
	_ = binary.Write(&b, binary.BigEndian, uint32(len(ihdr)))
	b.WriteString("IHDR")
	b.Write(ihdr)
	_ = binary.Write(&b, binary.BigEndian, crc32.ChecksumIEEE(append([]byte("IHDR"), ihdr...)))
	return b.Bytes()
}

func TestProcessUpload_JPEGAppliesOrientationAndDropsEXIF(t *testing.T) {
	in := withEXIF(encodeJPEG(t, halves(40, 20)), 6) // 6 = rotate 90° clockwise

	out, err := domain.ProcessUpload(in)
	require.NoError(t, err)
	require.Equal(t, "image/jpeg", out.ContentType)
	require.Equal(t, 20, out.Width)
	require.Equal(t, 40, out.Height)
	require.NotContains(t, string(out.Data), "Exif", "EXIF (and its GPS block) must be gone")

	img, err := jpeg.Decode(bytes.NewReader(out.Data))
	require.NoError(t, err)
	r, _, b, _ := img.At(10, 5).RGBA()
	require.Greater(t, r, b, "after a 90° clockwise turn the red left half is on top")
}

func TestProcessUpload_DownscalesWideImages(t *testing.T) {
	out, err := domain.ProcessUpload(encodePNG(t, halves(2400, 100)))
	require.NoError(t, err)
	require.Equal(t, "image/png", out.ContentType)
	require.Equal(t, domain.MaxImageWidth, out.Width)
	require.Equal(t, 50, out.Height)
}

func TestProcessUpload_KeepsSmallImageSize(t *testing.T) {
	out, err := domain.ProcessUpload(encodePNG(t, halves(300, 100)))
	require.NoError(t, err)
	require.Equal(t, 300, out.Width)
	require.Equal(t, 100, out.Height)
}

func TestProcessUpload_WebPBecomesJPEGOrPNG(t *testing.T) {
	for _, tc := range []struct{ name, b64, want string }{
		{"opaque", webpLossy1x1, "image/jpeg"},
		{"alpha", webpAlpha1x1, "image/png"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in, err := base64.StdEncoding.DecodeString(tc.b64)
			require.NoError(t, err)
			out, err := domain.ProcessUpload(in)
			require.NoError(t, err)
			require.Equal(t, tc.want, out.ContentType)
		})
	}
}

func TestProcessUpload_StoresGIFAsIs(t *testing.T) {
	var b bytes.Buffer
	frame := image.NewPaletted(image.Rect(0, 0, 3, 2), color.Palette{color.Black, color.White})
	require.NoError(t, gif.Encode(&b, frame, nil))

	out, err := domain.ProcessUpload(b.Bytes())
	require.NoError(t, err)
	require.Equal(t, b.Bytes(), out.Data, "GIFs keep their bytes so animations survive")
	require.Equal(t, 3, out.Width)
	require.Equal(t, 2, out.Height)
}

func TestProcessUpload_RejectsBombBeforeDecode(t *testing.T) {
	_, err := domain.ProcessUpload(pngHeader(20000, 20000))
	require.ErrorIs(t, err, domain.ErrImageTooLarge)
}

func TestProcessUpload_RejectsSVGAndCorruptImages(t *testing.T) {
	for _, in := range [][]byte{
		[]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		[]byte("\x89PNG\r\n\x1a\ntruncated"),
		[]byte("MZ\x90\x00 not an image"),
	} {
		_, err := domain.ProcessUpload(in)
		require.ErrorIs(t, err, domain.ErrUnsupportedMediaType, "%q", in)
	}
}

func TestProcessUpload_StoresPDFAsIs(t *testing.T) {
	in := []byte("%PDF-1.4\n%fake\n")
	out, err := domain.ProcessUpload(in)
	require.NoError(t, err)
	require.Equal(t, "application/pdf", out.ContentType)
	require.Equal(t, in, out.Data)
	require.Zero(t, out.Width)
}

func TestFilenameFor(t *testing.T) {
	for _, tc := range []struct{ in, contentType, want string }{
		{"photo.webp", "image/png", "photo.png"},
		{"photo.JPEG", "image/jpeg", "photo.JPEG"},
		{"photo.jpg", "image/jpeg", "photo.jpg"},
		{"scan", "application/pdf", "scan.pdf"},
		{"anim.gif", "image/gif", "anim.gif"},
	} {
		require.Equal(t, tc.want, domain.FilenameFor(tc.in, tc.contentType), tc.in)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/media/domain/`
Expected: FAIL to compile with `undefined: domain.ProcessUpload`.

- [ ] **Step 4: Implement**

Add to the `var (...)` block in `internal/media/domain/errors.go`:

```go
	// ErrImageTooLarge is returned when an image's header declares more
	// pixels than the decode budget. It is checked before any pixel buffer is
	// allocated, so a decompression bomb never reaches the decoder.
	ErrImageTooLarge = apperr.NewIncorrectInput("image_too_large",
		"this image has too many pixels (the limit is 25 megapixels)")
```

Create `internal/media/domain/imaging.go`:

```go
package domain

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"net/http"
	"path"
	"slices"
	"strings"

	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

const (
	// MaxImageWidth is twice the 600 px email content width, so images stay
	// sharp on high-density screens.
	MaxImageWidth = 1200
	// maxImagePixels bounds decoding: about 100 MB of pixels at 4 bytes each.
	maxImagePixels = 25_000_000
	jpegQuality    = 85
)

// Processed is an upload after server-side type detection and image
// normalization — the bytes the library stores.
type Processed struct {
	Data        []byte
	ContentType string
	// Width and Height are the stored image's pixel size; 0 for PDF.
	Width, Height int
}

type codec struct {
	config func(io.Reader) (image.Config, error)
	decode func(io.Reader) (image.Image, error)
}

var codecs = map[string]codec{
	"image/png":  {png.DecodeConfig, png.Decode},
	"image/jpeg": {jpeg.DecodeConfig, jpeg.Decode},
	"image/gif":  {gif.DecodeConfig, gif.Decode},
	"image/webp": {webp.DecodeConfig, webp.Decode},
}

// ProcessUpload detects data's type from its bytes and normalizes images for
// email. JPEG, PNG, and WebP are decoded, turned upright per EXIF, downscaled
// to MaxImageWidth, and re-encoded, which drops all metadata (EXIF, GPS).
// WebP becomes PNG when it has transparency and JPEG otherwise, because
// Outlook desktop cannot show WebP. GIF and PDF are stored as-is. Anything
// else, SVG included, is ErrUnsupportedMediaType.
func ProcessUpload(data []byte) (Processed, error) {
	ct := http.DetectContentType(data)
	if ct == "application/pdf" {
		return Processed{Data: data, ContentType: ct}, nil
	}
	c, ok := codecs[ct]
	if !ok {
		return Processed{}, ErrUnsupportedMediaType
	}
	cfg, err := c.config(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return Processed{}, ErrUnsupportedMediaType
	}
	if int64(cfg.Width)*int64(cfg.Height) > maxImagePixels {
		return Processed{}, ErrImageTooLarge
	}
	if ct == "image/gif" {
		// ponytail: GIFs are stored as-is so animations survive; a GIF wider
		// than MaxImageWidth relies on the renderer's width attribute.
		return Processed{Data: data, ContentType: ct, Width: cfg.Width, Height: cfg.Height}, nil
	}
	img, err := c.decode(bytes.NewReader(data))
	if err != nil {
		return Processed{}, ErrUnsupportedMediaType
	}

	orientation := 1
	if ct == "image/jpeg" {
		orientation = jpegOrientation(data)
	}
	// Downscale before rotating (cheaper), measuring the width the image will
	// have once it is upright.
	displayWidth := img.Bounds().Dx()
	if orientation >= 5 {
		displayWidth = img.Bounds().Dy()
	}
	img = downscale(img, float64(MaxImageWidth)/float64(displayWidth))
	img = applyOrientation(img, orientation)

	outType := ct
	if ct == "image/webp" {
		outType = "image/jpeg"
		if o, ok := img.(interface{ Opaque() bool }); ok && !o.Opaque() {
			outType = "image/png"
		}
	}
	var buf bytes.Buffer
	if outType == "image/jpeg" {
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality})
	} else {
		err = (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, img)
	}
	if err != nil {
		return Processed{}, fmt.Errorf("encoding image: %w", err)
	}
	b := img.Bounds()
	return Processed{Data: buf.Bytes(), ContentType: outType, Width: b.Dx(), Height: b.Dy()}, nil
}

// typeExtensions lists the filename extensions each stored type may carry;
// the first is the one FilenameFor appends.
var typeExtensions = map[string][]string{
	"image/png":       {".png"},
	"image/jpeg":      {".jpg", ".jpeg"},
	"image/gif":       {".gif"},
	"application/pdf": {".pdf"},
}

// FilenameFor gives filename the extension of contentType when it carries
// another one (a WebP converted to PNG, a mislabelled upload), so a
// downloaded file opens with the right program.
func FilenameFor(filename, contentType string) string {
	exts := typeExtensions[contentType]
	ext := path.Ext(filename)
	if len(exts) == 0 || slices.Contains(exts, strings.ToLower(ext)) {
		return filename
	}
	return strings.TrimSuffix(filename, ext) + exts[0]
}

// downscale resizes img by factor (< 1), keeping the aspect ratio.
func downscale(img image.Image, factor float64) image.Image {
	if factor >= 1 {
		return img
	}
	b := img.Bounds()
	w := max(1, int(math.Round(float64(b.Dx())*factor)))
	h := max(1, int(math.Round(float64(b.Dy())*factor)))
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}

// applyOrientation turns img upright according to an EXIF orientation value
// (1–8). Without it, phone photos come out rotated once EXIF is gone.
func applyOrientation(img image.Image, o int) image.Image {
	if o < 2 || o > 8 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	dst := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var dx, dy int
			switch o {
			case 2: // mirror horizontal
				dx, dy = w-1-x, y
			case 3: // rotate 180
				dx, dy = w-1-x, h-1-y
			case 4: // mirror vertical
				dx, dy = x, h-1-y
			case 5: // transpose
				dx, dy = y, x
			case 6: // rotate 90 clockwise
				dx, dy = h-1-y, x
			case 7: // transverse
				dx, dy = h-1-y, w-1-x
			case 8: // rotate 90 counter-clockwise
				dx, dy = y, w-1-x
			}
			dst.Set(dx, dy, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

// jpegOrientation returns the EXIF orientation (1–8) of a JPEG, or 1 when
// it has none. It walks the marker segments up to the start of scan and
// reads only the Orientation tag of IFD0.
// ponytail: no general EXIF parser; add one if more tags are ever needed.
func jpegOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 1
	}
	for i := 2; i+4 <= len(data); {
		if data[i] != 0xFF {
			return 1
		}
		marker := data[i+1]
		if marker == 0xDA || marker == 0xD9 { // start of scan, end of image
			return 1
		}
		size := int(binary.BigEndian.Uint16(data[i+2:]))
		if size < 2 || i+2+size > len(data) {
			return 1
		}
		seg := data[i+4 : i+2+size]
		if marker == 0xE1 && len(seg) >= 6 && string(seg[:6]) == "Exif\x00\x00" {
			return tiffOrientation(seg[6:])
		}
		i += 2 + size
	}
	return 1
}

// tiffOrientation reads tag 0x0112 from IFD0 of a TIFF block.
func tiffOrientation(t []byte) int {
	if len(t) < 8 {
		return 1
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 1
	}
	ifd := int(bo.Uint32(t[4:]))
	if ifd < 8 || ifd+2 > len(t) {
		return 1
	}
	n := int(bo.Uint16(t[ifd:]))
	for k := 0; k < n; k++ {
		e := ifd + 2 + k*12
		if e+12 > len(t) {
			return 1
		}
		if bo.Uint16(t[e:]) == 0x0112 {
			if v := int(bo.Uint16(t[e+8:])); v >= 1 && v <= 8 {
				return v
			}
			return 1
		}
	}
	return 1
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go mod tidy && go test ./internal/media/domain/ && go vet ./internal/media/...`
Expected: PASS; `golang.org/x/image` is now a direct requirement in `go.mod`.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/media/domain/imaging.go internal/media/domain/imaging_test.go internal/media/domain/errors.go
git commit -m "feat(media): detect upload types from bytes and normalize images for email"
```

---

### Task 2: Library URL format

**Files:**
- Create: `internal/media/domain/library_url.go`
- Create: `internal/media/domain/library_url_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `domain.LibraryURL(baseURL, tenantSlug, assetID string) string`.
  - `domain.NewLibraryURLs(baseURL string) domain.LibraryURLs`, with these methods:
    - `Is(ref string) bool`: an exact library URL under `baseURL`.
    - `AssetIDs(texts ...string) []string`: distinct ids, first-seen order.
    - `Replace(text string, to func(assetID string) string) string`.

- [ ] **Step 1: Write the failing tests**

Create `internal/media/domain/library_url_test.go`:

```go
package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/media/domain"
)

const (
	testBase  = "https://app.test"
	assetA    = "0b9f3c3e-6a7d-4c1e-9a55-2f1d7e4b8c90"
	assetB    = "1c2d3e4f-0000-4000-8000-000000000001"
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
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/media/domain/ -run LibraryURL`
Expected: FAIL to compile with `undefined: domain.LibraryURL`.

- [ ] **Step 3: Implement**

Create `internal/media/domain/library_url.go`:

```go
package domain

import (
	"regexp"
	"strings"
)

// libraryPath is the path of a library URL after BASE_URL: the tenant slug
// (group 1, the tenant context's slug rule) and the asset id (group 2, a
// lowercase UUID).
const libraryPath = `/t/([a-z0-9][a-z0-9-]{1,61}[a-z0-9])/api/media/` +
	`([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})/content`

// LibraryURL returns the URL content stores for a library asset. It points
// at the app origin: for a workspace member, GET answers with a redirect to
// a short-lived presigned URL, and publishing rewrites it to the public copy.
func LibraryURL(baseURL, tenantSlug, assetID string) string {
	return strings.TrimRight(baseURL, "/") + "/t/" + tenantSlug + "/api/media/" + assetID + "/content"
}

// LibraryURLs finds library URLs under one BASE_URL. Library URLs contain
// nothing that HTML or CSS escape, so one pattern matches them in src, href,
// background, and url() alike.
type LibraryURLs struct {
	anywhere *regexp.Regexp
	exact    *regexp.Regexp
}

// NewLibraryURLs builds the matcher for baseURL.
func NewLibraryURLs(baseURL string) LibraryURLs {
	prefix := regexp.QuoteMeta(strings.TrimRight(baseURL, "/"))
	return LibraryURLs{
		anywhere: regexp.MustCompile(prefix + libraryPath),
		exact:    regexp.MustCompile(`^` + prefix + libraryPath + `$`),
	}
}

// Is reports whether ref is exactly one library URL.
func (l LibraryURLs) Is(ref string) bool { return l.exact.MatchString(ref) }

// AssetIDs returns the distinct asset ids referenced across texts, in
// first-seen order.
func (l LibraryURLs) AssetIDs(texts ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range texts {
		for _, m := range l.anywhere.FindAllStringSubmatch(t, -1) {
			if !seen[m[2]] {
				seen[m[2]] = true
				out = append(out, m[2])
			}
		}
	}
	return out
}

// Replace rewrites every library URL in text to to(assetID).
func (l LibraryURLs) Replace(text string, to func(assetID string) string) string {
	return l.anywhere.ReplaceAllStringFunc(text, func(m string) string {
		return to(l.anywhere.FindStringSubmatch(m)[2])
	})
}
```

- [ ] **Step 4: Run to verify they pass**

Run: `go test ./internal/media/domain/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/media/domain/library_url.go internal/media/domain/library_url_test.go
git commit -m "feat(media): define the app-origin library URL format"
```

---

### Task 3: Private blob store with publish and presign

**Files:**
- Modify: `internal/media/domain/blobstore.go` (replace the interface)
- Modify: `internal/media/adapters/blobstore_s3.go`
- Modify: `internal/media/adapters/blobstore_memory.go`
- Modify: `internal/media/adapters/blobstore_s3_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces `domain.BlobStore` with these methods:

```go
Put(ctx context.Context, key, contentType string, contentLength int64, body io.Reader) error
Delete(ctx context.Context, key string) error
Publish(ctx context.Context, srcKey, dstKey, contentType, disposition string) error
PresignGet(ctx context.Context, key string, ttl time.Duration, disposition string) (string, error)
PublicURL(key string) string
BuildKey(tenantID, id, filename string) string
PublishedKey(tenantID, id, filename string) string
```

- `MemoryBlobStore` keeps `Get(key) ([]byte, string, bool)` and `Len()`, and adds:
  - `DispositionOf(key) string`.
  - `IsPublic(key) bool`.
  - A `PublishErr error` field: the next `Publish` fails.
  - `PresignGet`, which returns `"{base}/{key}?signature=test"`.

- [ ] **Step 1: Write the failing S3 tests**

In `internal/media/adapters/blobstore_s3_test.go`:

1. In `ensureBucket`, change the policy resource from `/media/*` to `/published/*`. Update its comment to: "grants anonymous read on the published/ prefix only — the same shape production uses: library objects stay private, public copies are readable."
2. Replace `TestS3BlobStore_PutGetDelete` with the two tests below.
3. Add the `PublishedKey` assertion to `TestS3BlobStore_KeysAreTenantPrefixed`.

```go
func TestS3BlobStore_LibraryObjectsArePrivate(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	key := store.BuildKey("tenant-a", "asset-1", "hello.png")
	body := []byte("hello-png-bytes")
	require.NoError(t, store.Put(ctx, key, "image/png", int64(len(body)), bytes.NewReader(body)))

	resp, err := http.Get(store.PublicURL(key))
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusForbidden, resp.StatusCode,
		"a library object must not be anonymously readable")

	signed, err := store.PresignGet(ctx, key, time.Minute, "inline")
	require.NoError(t, err)
	resp, err = http.Get(signed)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "inline", resp.Header.Get("Content-Disposition"))
	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, body, got)

	require.NoError(t, store.Delete(ctx, key))
}

func TestS3BlobStore_PublishMakesAPublicCopy(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	src := store.BuildKey("tenant-a", "asset-2", "price list.pdf")
	dst := store.PublishedKey("tenant-a", "asset-2", "price list.pdf")
	body := []byte("%PDF-1.4 fake")
	require.NoError(t, store.Put(ctx, src, "application/pdf", int64(len(body)), bytes.NewReader(body)))

	const disposition = `attachment; filename="price list.pdf"`
	require.NoError(t, store.Publish(ctx, src, dst, "application/pdf", disposition))
	require.NoError(t, store.Publish(ctx, src, dst, "application/pdf", disposition),
		"publishing again overwrites the copy")

	resp, err := http.Get(store.PublicURL(dst))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/pdf", resp.Header.Get("Content-Type"))
	require.Equal(t, disposition, resp.Header.Get("Content-Disposition"))
	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, body, got)

	require.NoError(t, store.Delete(ctx, src))
	again, err := http.Get(store.PublicURL(dst))
	require.NoError(t, err)
	require.NoError(t, again.Body.Close())
	require.Equal(t, http.StatusOK, again.StatusCode, "the public copy outlives the library object")
	require.NoError(t, store.Delete(ctx, dst))
}
```

Add to `TestS3BlobStore_KeysAreTenantPrefixed`:

```go
	require.Equal(t, "published/tenant-a/asset-1/file.png",
		store.PublishedKey("tenant-a", "asset-1", "file.png"))
```

- [ ] **Step 2: Run to verify they fail**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock"; go test ./internal/media/adapters/ -run S3`
Expected: FAIL to compile: `store.PresignGet undefined`.

- [ ] **Step 3: Replace the port**

Replace the body of `internal/media/domain/blobstore.go`:

```go
package domain

import (
	"context"
	"io"
	"time"
)

// BlobStore is the consumer-owned port through which the media context
// stores object bytes. Library objects are private; an asset becomes
// publicly readable only through Publish, which writes a separate copy.
// The S3 adapter and the in-memory fake both implement it.
type BlobStore interface {
	// Put writes body to key as a private object. It must return only after
	// the write has been durably accepted by the store.
	Put(ctx context.Context, key, contentType string, contentLength int64, body io.Reader) error
	// Delete removes the object at key. Deleting an absent key is not an
	// error — the metadata row is the source of truth.
	Delete(ctx context.Context, key string) error
	// Publish copies srcKey to dstKey server-side as a publicly readable
	// object served with the given Content-Type and Content-Disposition.
	// Copying onto an existing dstKey overwrites it, so it is idempotent.
	Publish(ctx context.Context, srcKey, dstKey, contentType, disposition string) error
	// PresignGet returns a URL that reads key until ttl elapses, answered
	// with the given Content-Disposition.
	PresignGet(ctx context.Context, key string, ttl time.Duration, disposition string) (string, error)
	// PublicURL returns the public URL of a published key.
	PublicURL(key string) string
	// BuildKey returns the private library key for an asset. The shape —
	// tenant-prefixed and carrying an unguessable id — is part of the
	// isolation strategy.
	BuildKey(tenantID, id, filename string) string
	// PublishedKey returns the key of an asset's public copy.
	PublishedKey(tenantID, id, filename string) string
}
```

- [ ] **Step 4: Update the S3 adapter**

In `internal/media/adapters/blobstore_s3.go`:

- Add `"time"` to the imports.
- Add a `presign *s3.PresignClient` field to `S3BlobStore`. Set it in `NewS3BlobStore` with `presign: s3.NewPresignClient(client)`.
- In `Put`, delete the `ACL: types.ObjectCannedACLPublicRead,` line and change the doc comment to "Put writes body to key as a private object…".
- Replace `PublicURL` and append the new methods:

```go
// escapeKey URL-escapes each key segment so a filename with spaces or
// unicode round-trips in URLs and CopySource headers.
func escapeKey(key string) string {
	parts := strings.Split(key, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

// PublicURL returns the public URL of a published key.
func (s *S3BlobStore) PublicURL(key string) string {
	return s.publicBaseURL + "/" + escapeKey(key)
}

// Publish copies srcKey to dstKey server-side with a public-read ACL and the
// given serving headers. REPLACE makes the copy carry contentType and
// disposition rather than the source's metadata.
func (s *S3BlobStore) Publish(ctx context.Context, srcKey, dstKey, contentType, disposition string) error {
	_, err := s.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:             aws.String(s.bucket),
		Key:                aws.String(dstKey),
		CopySource:         aws.String(s.bucket + "/" + escapeKey(srcKey)),
		MetadataDirective:  types.MetadataDirectiveReplace,
		ContentType:        aws.String(contentType),
		ContentDisposition: aws.String(disposition),
		ACL:                types.ObjectCannedACLPublicRead,
	})
	if err != nil {
		return fmt.Errorf("publishing %s: %w", srcKey, err)
	}
	return nil
}

// PresignGet returns a SigV4-presigned GET for key, valid for ttl.
func (s *S3BlobStore) PresignGet(ctx context.Context, key string, ttl time.Duration,
	disposition string) (string, error) {
	req, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket:                     aws.String(s.bucket),
		Key:                        aws.String(key),
		ResponseContentDisposition: aws.String(disposition),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("presigning %s: %w", key, err)
	}
	return req.URL, nil
}

// PublishedKey returns the key of an asset's public copy.
func (s *S3BlobStore) PublishedKey(tenantID, id, filename string) string {
	return "published/" + tenantID + "/" + id + "/" + filename
}
```

- [ ] **Step 5: Update the in-memory fake**

Replace `internal/media/adapters/blobstore_memory.go` below the imports (add `"time"` to the imports):

```go
// MemoryBlobStore is an in-memory implementation of domain.BlobStore used by
// use-case and API tests, so they exercise upload, publish, and delete
// without booting an object store.
type MemoryBlobStore struct {
	mu            sync.Mutex
	objects       map[string]memoryObject
	publicBaseURL string
	// PutErr, when non-nil, makes the next Put fail — used by tests to assert
	// that the metadata row is not written when the upload fails.
	PutErr error
	// PublishErr, when non-nil, makes the next Publish fail.
	PublishErr error
}

type memoryObject struct {
	data        []byte
	contentType string
	disposition string
	public      bool
}

var _ domain.BlobStore = (*MemoryBlobStore)(nil)

// NewMemoryBlobStore builds an empty MemoryBlobStore that builds public URLs
// from publicBaseURL (any trailing slash is trimmed).
func NewMemoryBlobStore(publicBaseURL string) *MemoryBlobStore {
	return &MemoryBlobStore{
		objects:       map[string]memoryObject{},
		publicBaseURL: strings.TrimRight(publicBaseURL, "/"),
	}
}

// Put stores body's bytes as a private object under key, returning PutErr
// first if a test has staged a failure.
func (s *MemoryBlobStore) Put(_ context.Context, key, contentType string,
	_ int64, body io.Reader) error {
	if s.PutErr != nil {
		err := s.PutErr
		s.PutErr = nil
		return err
	}
	buf := &bytes.Buffer{}
	if _, err := io.Copy(buf, body); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = memoryObject{data: buf.Bytes(), contentType: contentType}
	return nil
}

// Delete removes the object at key. Deleting an absent key is a no-op.
func (s *MemoryBlobStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, key)
	return nil
}

// Publish copies srcKey to a public dstKey, returning PublishErr first if a
// test has staged a failure.
func (s *MemoryBlobStore) Publish(_ context.Context, srcKey, dstKey, contentType, disposition string) error {
	if s.PublishErr != nil {
		err := s.PublishErr
		s.PublishErr = nil
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	src, ok := s.objects[srcKey]
	if !ok {
		return fmt.Errorf("publishing %s: no such object", srcKey)
	}
	s.objects[dstKey] = memoryObject{data: src.data, contentType: contentType,
		disposition: disposition, public: true}
	return nil
}

// PresignGet returns a recognisable fake signed URL.
func (s *MemoryBlobStore) PresignGet(_ context.Context, key string, _ time.Duration, _ string) (string, error) {
	return s.publicBaseURL + "/" + key + "?signature=test", nil
}

// PublicURL builds the public URL for key by joining publicBaseURL with it.
func (s *MemoryBlobStore) PublicURL(key string) string {
	return s.publicBaseURL + "/" + key
}

// BuildKey mirrors the production layout.
func (s *MemoryBlobStore) BuildKey(tenantID, id, filename string) string {
	return "media/" + tenantID + "/" + id + "/" + filename
}

// PublishedKey mirrors the production layout.
func (s *MemoryBlobStore) PublishedKey(tenantID, id, filename string) string {
	return "published/" + tenantID + "/" + id + "/" + filename
}

// Get returns the stored bytes and content type for key, reporting whether
// the object exists.
func (s *MemoryBlobStore) Get(key string) ([]byte, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.objects[key]
	return o.data, o.contentType, ok
}

// DispositionOf returns the Content-Disposition a published key is served with.
func (s *MemoryBlobStore) DispositionOf(key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.objects[key].disposition
}

// IsPublic reports whether key is a published (publicly readable) object.
func (s *MemoryBlobStore) IsPublic(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.objects[key].public
}

// Len returns how many objects are stored, for tests asserting no leftover.
func (s *MemoryBlobStore) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.objects)
}
```

Add `"fmt"` to its imports.

- [ ] **Step 6: Run the tests**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock"; go build ./... && go test ./internal/media/...`
Expected: PASS. The upload command still compiles, because it uses only `Put`, `Delete`, `BuildKey`, and `PublicURL`.

- [ ] **Step 7: Commit**

```bash
git add internal/media/domain/blobstore.go internal/media/adapters/
git commit -m "feat(media): store library objects privately, publish copies, presign reads"
```

---

### Task 4: Private library end to end (model, migration, upload, list, content endpoint)

**Files:**
- Create: `internal/db/migrations/000024_private_media.up.sql`, `internal/db/migrations/000024_private_media.down.sql`
- Modify: `test/migrate_test.go` (expect version 24)
- Modify: `internal/media/domain/asset.go`, `internal/media/domain/repository.go`
- Create: `internal/media/domain/asset_test.go`
- Modify: `internal/media/adapters/assets_pg.go`, `internal/media/adapters/assets_pg_test.go`
- Modify: `internal/media/app/command/upload_asset.go`, `internal/media/app/command/upload_asset_test.go`
- Modify: `internal/media/app/query/list_assets.go`
- Create: `internal/media/app/query/asset_content.go`
- Modify: `internal/media/app/application.go`
- Modify: `internal/service/application.go` (`buildMedia`)
- Modify: `internal/api/media_handlers.go`, `internal/api/server.go`
- Create: `internal/api/media_handlers_test.go`

**Interfaces:**
- Consumes: from Task 1, `ProcessUpload`, `FilenameFor`, and `ErrImageTooLarge`; from Task 2, `LibraryURL`; from Task 3, `BlobStore`.
- Produces:
  - `NewMediaAsset(id, tenantID, filename, contentType string, sizeBytes, maxBytes int64, width, height int, storageKey, uploadedBy string) (*MediaAsset, error)`.
  - `HydrateMediaAsset(id, tenantID, filename, contentType string, sizeBytes int64, width, height int, storageKey, uploadedBy string, publishedAt *time.Time, createdAt time.Time) *MediaAsset`.
  - Accessors `Width() int`, `Height() int`, `IsPublished() bool`, and `Disposition() string`. `PublicURL()` is removed.
  - `domain.CleanFilename(filename string) (string, error)`.
  - `MediaRepository.MarkPublished(ctx, tenantID, id string, at time.Time) error`.
  - `command.UploadAsset{TenantID, TenantSlug, Filename string; Body io.Reader; UploadedBy string}`.
  - `command.UploadAssetResult{AssetID, URL, Filename string; Width, Height int}`.
  - `command.NewUploadAssetHandler(assets, blobs, maxBytes int64, baseURL string)`.
  - `query.ListAssets{TenantID, TenantSlug}`.
  - `query.AssetView` JSON fields: `id`, `filename`, `content_type`, `size_bytes`, `url`, `preview_url`, `width`, `height`, `published`, `uploaded_by`, `created_at`.
  - `query.NewListAssetsHandler(assets, blobs, baseURL string)`.
  - `query.AssetContentURL{TenantID, AssetID}`, whose handler returns the presigned URL as a `string`.
  - `query.NewAssetContentURLHandler(assets, blobs)`.
  - `query.PreviewTTL = time.Hour` and `query.ContentURLTTL = 15 * time.Minute`.
  - `mediaapp.Queries.AssetContentURL`.
  - HTTP: `GET /t/{slug}/api/media/{id}/content` answers 302.
  - The test helpers `testPNG(t, w, h) []byte`, `(*testServer).uploadMedia(slug, filename string, data []byte) (int, map[string]any)`, and `noRedirects(*http.Client) *http.Client` in `internal/api/media_handlers_test.go`.

- [ ] **Step 1: Migration**

Create `internal/db/migrations/000024_private_media.up.sql`:

```sql
-- Track B: the media library is private. The public URL is no longer a
-- property of an asset (content stores an app-origin library URL; public
-- copies are made at send time), so public_url goes. width/height feed the
-- email renderer's width attribute; published_at marks that the asset's
-- public copy exists.
ALTER TABLE media_assets
    DROP COLUMN public_url,
    ADD COLUMN width        int CHECK (width > 0),
    ADD COLUMN height       int CHECK (height > 0),
    ADD COLUMN published_at timestamptz;
```

Create `internal/db/migrations/000024_private_media.down.sql`:

```sql
ALTER TABLE media_assets
    DROP COLUMN published_at,
    DROP COLUMN height,
    DROP COLUMN width,
    ADD COLUMN public_url text NOT NULL DEFAULT '';
```

In `test/migrate_test.go`, change the comment `reaches 23` to `reaches 24` and `uint(23)` to `uint(24)`.

- [ ] **Step 2: Write the failing domain test**

Create `internal/media/domain/asset_test.go`:

```go
package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/media/domain"
)

func TestNewMediaAsset_imagesNeedDimensions(t *testing.T) {
	_, err := domain.NewMediaAsset("id", "t1", "a.png", "image/png", 10, 100, 0, 0, "media/t1/id/a.png", "")
	require.Error(t, err)

	pdf, err := domain.NewMediaAsset("id", "t1", "a.pdf", "application/pdf", 10, 100, 0, 0, "media/t1/id/a.pdf", "")
	require.NoError(t, err)
	require.Zero(t, pdf.Width())
	require.False(t, pdf.IsPublished())
}

func TestNewMediaAsset_rejectsSVG(t *testing.T) {
	_, err := domain.NewMediaAsset("id", "t1", "a.svg", "image/svg+xml", 10, 100, 5, 5, "media/t1/id/a.svg", "")
	require.ErrorIs(t, err, domain.ErrUnsupportedMediaType)
}

func TestMediaAsset_Disposition(t *testing.T) {
	img, err := domain.NewMediaAsset("id", "t1", "a.png", "image/png", 10, 100, 5, 5, "k", "")
	require.NoError(t, err)
	require.Equal(t, "inline", img.Disposition())

	pdf, err := domain.NewMediaAsset("id", "t1", "price list.pdf", "application/pdf", 10, 100, 0, 0, "k", "")
	require.NoError(t, err)
	require.Equal(t, `attachment; filename="price list.pdf"`, pdf.Disposition())

	ru, err := domain.NewMediaAsset("id", "t1", "прайс.pdf", "application/pdf", 10, 100, 0, 0, "k", "")
	require.NoError(t, err)
	require.Equal(t, "attachment; filename*=utf-8''%D0%BF%D1%80%D0%B0%D0%B9%D1%81.pdf", ru.Disposition())
}
```

Run: `go test ./internal/media/domain/ -run MediaAsset`
Expected: FAIL to compile (wrong `NewMediaAsset` arity, no `Disposition`).

- [ ] **Step 3: Rewrite the asset model**

Replace `internal/media/domain/asset.go` (keep the package doc comment):

```go
package domain

import (
	"mime"
	"path"
	"strings"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

// allowedContentTypes is the set of types the library stores. Uploads are
// detected from their bytes (ProcessUpload), and WebP is stored converted,
// so it never appears here. SVG is deliberately absent: mail clients do not
// render it, and opened directly it can run script.
var allowedContentTypes = map[string]bool{
	"image/png":       true,
	"image/jpeg":      true,
	"image/gif":       true,
	"application/pdf": true,
}

// MediaAsset is one file in a tenant's media library. Metadata (this row) is
// tenant-scoped under RLS; the bytes live in a private object at a
// tenant-prefixed, unguessable key. A public copy exists only once content
// using the asset has gone live.
type MediaAsset struct {
	id          string
	tenantID    string
	filename    string
	contentType string
	sizeBytes   int64
	width       int
	height      int
	storageKey  string
	uploadedBy  string
	publishedAt *time.Time
	createdAt   time.Time
}

// CleanFilename strips any directory component a client may have sent and
// rejects an empty result. It does not enforce a character allowlist — the
// filename is only written into the object key (URL-encoded by the SDK) and
// shown as a display string.
func CleanFilename(filename string) (string, error) {
	cleaned := path.Base(strings.TrimSpace(filename))
	if cleaned == "" || cleaned == "." || cleaned == "/" {
		return "", ErrEmptyFilename
	}
	return cleaned, nil
}

// NewMediaAsset builds a media asset, rejecting any invariant violation. The
// id is generated by the caller so it can be embedded in the storage key
// before the bytes are written. maxBytes is the configured per-file cap.
// Images carry their pixel size; other files carry 0×0.
func NewMediaAsset(id, tenantID, filename, contentType string, sizeBytes, maxBytes int64,
	width, height int, storageKey, uploadedBy string) (*MediaAsset, error) {

	if id == "" {
		return nil, apperr.NewIncorrectInput("validation_failed", "a media asset id is required")
	}
	if tenantID == "" {
		return nil, apperr.NewIncorrectInput("validation_failed", "a tenant is required")
	}
	cleanName, err := CleanFilename(filename)
	if err != nil {
		return nil, err
	}
	if !allowedContentTypes[contentType] {
		return nil, ErrUnsupportedMediaType
	}
	if sizeBytes <= 0 {
		return nil, ErrEmptyUpload
	}
	if sizeBytes > maxBytes {
		return nil, ErrMediaTooLarge
	}
	if strings.HasPrefix(contentType, "image/") && (width <= 0 || height <= 0) {
		return nil, apperr.NewIncorrectInput("validation_failed", "an image needs its dimensions")
	}
	if storageKey == "" {
		return nil, apperr.NewIncorrectInput("validation_failed", "a storage key is required")
	}
	return &MediaAsset{
		id: id, tenantID: tenantID, filename: cleanName, contentType: contentType,
		sizeBytes: sizeBytes, width: width, height: height,
		storageKey: storageKey, uploadedBy: uploadedBy,
	}, nil
}

// HydrateMediaAsset reconstructs a media asset from a persisted row.
// Persistence only — it performs no validation.
func HydrateMediaAsset(id, tenantID, filename, contentType string, sizeBytes int64,
	width, height int, storageKey, uploadedBy string, publishedAt *time.Time,
	createdAt time.Time) *MediaAsset {
	return &MediaAsset{
		id: id, tenantID: tenantID, filename: filename, contentType: contentType,
		sizeBytes: sizeBytes, width: width, height: height, storageKey: storageKey,
		uploadedBy: uploadedBy, publishedAt: publishedAt, createdAt: createdAt,
	}
}

// ID returns the asset's id (also the unguessable key segment).
func (a *MediaAsset) ID() string { return a.id }

// TenantID returns the owning tenant's id.
func (a *MediaAsset) TenantID() string { return a.tenantID }

// Filename returns the sanitised filename.
func (a *MediaAsset) Filename() string { return a.filename }

// ContentType returns the detected MIME type.
func (a *MediaAsset) ContentType() string { return a.contentType }

// SizeBytes returns the stored file size in bytes.
func (a *MediaAsset) SizeBytes() int64 { return a.sizeBytes }

// Width returns the image width in pixels, or 0 for a non-image.
func (a *MediaAsset) Width() int { return a.width }

// Height returns the image height in pixels, or 0 for a non-image.
func (a *MediaAsset) Height() int { return a.height }

// StorageKey returns the private object key.
func (a *MediaAsset) StorageKey() string { return a.storageKey }

// UploadedBy returns the user id that uploaded the asset, or "".
func (a *MediaAsset) UploadedBy() string { return a.uploadedBy }

// IsPublished reports whether the asset's public copy exists.
func (a *MediaAsset) IsPublished() bool { return a.publishedAt != nil }

// CreatedAt returns when the asset was uploaded.
func (a *MediaAsset) CreatedAt() time.Time { return a.createdAt }

// Disposition is the Content-Disposition the asset is served with: images
// display inline; anything else (PDF) downloads under its filename.
func (a *MediaAsset) Disposition() string {
	if strings.HasPrefix(a.contentType, "image/") {
		return "inline"
	}
	if d := mime.FormatMediaType("attachment", map[string]string{"filename": a.filename}); d != "" {
		return d
	}
	return "attachment"
}
```

(`AllowedContentTypes()` is deleted; nothing called it.)

Add to `MediaRepository` in `internal/media/domain/repository.go` (and add `"time"` to its imports):

```go
	// MarkPublished records that the asset's public copy exists. Marking an
	// already-published asset keeps the first timestamp.
	MarkPublished(ctx context.Context, tenantID, id string, at time.Time) error
```

- [ ] **Step 4: Update the pgx repository and its test**

In `internal/media/adapters/assets_pg.go`, replace `assetColumns`, `scanAssetRow`, and `Add`, and append `MarkPublished` and `nullIfZero`:

```go
const assetColumns = "id, tenant_id, filename, content_type, size_bytes, " +
	"COALESCE(width, 0), COALESCE(height, 0), storage_key, " +
	"COALESCE(uploaded_by::text, ''), published_at, created_at"

func scanAssetRow(row pgx.Row) (*domain.MediaAsset, error) {
	var id, tenantID, filename, contentType, storageKey, uploadedBy string
	var sizeBytes int64
	var width, height int
	var publishedAt *time.Time
	var createdAt time.Time
	if err := row.Scan(&id, &tenantID, &filename, &contentType, &sizeBytes,
		&width, &height, &storageKey, &uploadedBy, &publishedAt, &createdAt); err != nil {
		return nil, err
	}
	return domain.HydrateMediaAsset(id, tenantID, filename, contentType, sizeBytes,
		width, height, storageKey, uploadedBy, publishedAt, createdAt), nil
}

// Add persists a new media-asset row. The id is set by the caller so it can
// be embedded in the storage key before the bytes are written.
func (r *Assets) Add(ctx context.Context, a *domain.MediaAsset) error {
	return tenantdb.WithTenant(ctx, r.pool, a.TenantID(), func(ctx context.Context, tx pgx.Tx) error {
		var uploadedBy any
		if a.UploadedBy() != "" {
			uploadedBy = a.UploadedBy()
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO media_assets
			   (id, tenant_id, filename, content_type, size_bytes,
			    width, height, storage_key, uploaded_by)
			 VALUES (@id, @tenant_id, @filename, @content_type, @size_bytes,
			         @width, @height, @storage_key, @uploaded_by)`,
			pgx.NamedArgs{
				"id":           a.ID(),
				"tenant_id":    a.TenantID(),
				"filename":     a.Filename(),
				"content_type": a.ContentType(),
				"size_bytes":   a.SizeBytes(),
				"width":        nullIfZero(a.Width()),
				"height":       nullIfZero(a.Height()),
				"storage_key":  a.StorageKey(),
				"uploaded_by":  uploadedBy,
			})
		if err != nil {
			return fmt.Errorf("inserting media asset: %w", err)
		}
		return nil
	})
}

// MarkPublished records that the asset's public copy exists, keeping the
// first timestamp when it is already set.
func (r *Assets) MarkPublished(ctx context.Context, tenantID, id string, at time.Time) error {
	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			"UPDATE media_assets SET published_at = COALESCE(published_at, $2) WHERE id = $1", id, at)
		if db.IsInvalidInput(err) {
			return domain.ErrMediaNotFound
		}
		if err != nil {
			return fmt.Errorf("marking media asset published: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrMediaNotFound
		}
		return nil
	})
}

// nullIfZero stores a missing dimension (PDF) as NULL so the CHECK (> 0)
// constraints hold.
func nullIfZero(n int) any {
	if n == 0 {
		return nil
	}
	return n
}
```

In `internal/media/adapters/assets_pg_test.go`:

1. Change `newAsset` to:

```go
	a, err := domain.NewMediaAsset(id, tenantID, "logo.png", "image/png", 42, 1<<20, 64, 32,
		"media/"+tenantID+"/"+id+"/logo.png", "")
```

2. In `TestAssetsAddGetListDelete`, after the `SizeBytes` assertion, add:

```go
	require.Equal(t, 64, got.Width())
	require.Equal(t, 32, got.Height())
	require.False(t, got.IsPublished())
```

3. Append:

```go
func TestAssetsMarkPublished(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewAssets(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	a := newAsset(t, tenantID)
	require.NoError(t, repo.Add(ctx, a))
	require.NoError(t, repo.MarkPublished(ctx, tenantID, a.ID(), time.Now()))
	require.NoError(t, repo.MarkPublished(ctx, tenantID, a.ID(), time.Now()), "marking twice is fine")

	got, err := repo.Get(ctx, tenantID, a.ID())
	require.NoError(t, err)
	require.True(t, got.IsPublished())

	require.ErrorIs(t, repo.MarkPublished(ctx, tenantID, uuid.NewString(), time.Now()),
		domain.ErrMediaNotFound)
}

func TestAssetsStorePDFWithoutDimensions(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewAssets(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	id := uuid.NewString()
	pdf, err := domain.NewMediaAsset(id, tenantID, "a.pdf", "application/pdf", 42, 1<<20, 0, 0,
		"media/"+tenantID+"/"+id+"/a.pdf", "")
	require.NoError(t, err)
	require.NoError(t, repo.Add(ctx, pdf))
	got, err := repo.Get(ctx, tenantID, id)
	require.NoError(t, err)
	require.Zero(t, got.Width())
}
```

Add `"time"` to the test imports.

- [ ] **Step 5: Rewrite the upload command**

Replace `internal/media/app/command/upload_asset.go` (keep the package doc comment):

```go
package command

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/google/uuid"

	"github.com/nikolaymatrosov/nvelope/internal/media/domain"
)

// UploadAsset is the request to add one file to a tenant's media library.
// The content type is detected from the bytes; the client's claim is not
// used.
type UploadAsset struct {
	TenantID   string
	TenantSlug string
	Filename   string
	Body       io.Reader
	// UploadedBy carries the requesting user's id for audit; "" when absent.
	UploadedBy string
}

// UploadAssetResult describes the stored asset. URL is the library URL —
// what content embeds; only workspace members can read it until content
// that uses it is published.
type UploadAssetResult struct {
	AssetID  string
	URL      string
	Filename string
	Width    int
	Height   int
}

// UploadAssetHandler handles the UploadAsset command.
type UploadAssetHandler struct {
	assets   domain.MediaRepository
	blobs    domain.BlobStore
	maxBytes int64
	baseURL  string
}

// NewUploadAssetHandler builds the handler, failing fast on a nil dependency
// or a non-positive size cap. baseURL is the app origin library URLs use.
func NewUploadAssetHandler(assets domain.MediaRepository, blobs domain.BlobStore,
	maxBytes int64, baseURL string) UploadAssetHandler {
	if assets == nil || blobs == nil {
		panic("nil dependency")
	}
	if maxBytes <= 0 {
		panic("media upload size cap must be positive")
	}
	return UploadAssetHandler{assets: assets, blobs: blobs, maxBytes: maxBytes, baseURL: baseURL}
}

// decodeSlots bounds concurrent upload processing in this process: a 25 MP
// image holds about 100 MB of pixels while it is decoded.
// ponytail: per-process limit; move processing to a worker job if upload
// volume ever needs it.
var decodeSlots = make(chan struct{}, 2)

// Handle reads and normalizes the upload, writes the private object first,
// then persists the metadata row. The order matters: an interrupted upload
// leaves an orphaned object (cheap, invisible to the listing) rather than a
// listed-but-missing asset (FR-029).
func (h UploadAssetHandler) Handle(ctx context.Context, cmd UploadAsset) (UploadAssetResult, error) {
	name, err := domain.CleanFilename(cmd.Filename)
	if err != nil {
		return UploadAssetResult{}, err
	}
	data, err := io.ReadAll(io.LimitReader(cmd.Body, h.maxBytes+1))
	if err != nil {
		return UploadAssetResult{}, fmt.Errorf("reading upload: %w", err)
	}
	if len(data) == 0 {
		return UploadAssetResult{}, domain.ErrEmptyUpload
	}
	if int64(len(data)) > h.maxBytes {
		return UploadAssetResult{}, domain.ErrMediaTooLarge
	}
	processed, err := process(ctx, data)
	if err != nil {
		return UploadAssetResult{}, err
	}

	id := uuid.NewString()
	filename := domain.FilenameFor(name, processed.ContentType)
	asset, err := domain.NewMediaAsset(id, cmd.TenantID, filename, processed.ContentType,
		int64(len(processed.Data)), h.maxBytes, processed.Width, processed.Height,
		h.blobs.BuildKey(cmd.TenantID, id, filename), cmd.UploadedBy)
	if err != nil {
		return UploadAssetResult{}, err
	}
	if err := h.blobs.Put(ctx, asset.StorageKey(), asset.ContentType(),
		asset.SizeBytes(), bytes.NewReader(processed.Data)); err != nil {
		return UploadAssetResult{}, err
	}
	if err := h.assets.Add(ctx, asset); err != nil {
		// Best-effort cleanup so a metadata-insert failure leaves no orphan.
		_ = h.blobs.Delete(ctx, asset.StorageKey())
		return UploadAssetResult{}, err
	}
	return UploadAssetResult{
		AssetID:  asset.ID(),
		URL:      domain.LibraryURL(h.baseURL, cmd.TenantSlug, asset.ID()),
		Filename: asset.Filename(),
		Width:    asset.Width(),
		Height:   asset.Height(),
	}, nil
}

// process runs ProcessUpload while holding one of the decode slots.
func process(ctx context.Context, data []byte) (domain.Processed, error) {
	select {
	case decodeSlots <- struct{}{}:
	case <-ctx.Done():
		return domain.Processed{}, ctx.Err()
	}
	defer func() { <-decodeSlots }()
	return domain.ProcessUpload(data)
}
```

- [ ] **Step 6: Rewrite the list query and add the content query**

Replace everything below the imports in `internal/media/app/query/list_assets.go`. The imports are `context`, `time`, and the media domain.

```go
// PreviewTTL is how long a listing's preview URLs stay valid. The SPA
// refetches the listing well before (staleTime 30 min).
const PreviewTTL = time.Hour

// AssetView is the read model for one media asset, shaped for the admin API.
type AssetView struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	// URL is the library URL content embeds.
	URL string `json:"url"`
	// PreviewURL is a presigned read valid for PreviewTTL — for thumbnails
	// and the sandboxed preview iframe, which carries no session cookie.
	PreviewURL string    `json:"preview_url"`
	Width      int       `json:"width,omitempty"`
	Height     int       `json:"height,omitempty"`
	Published  bool      `json:"published"`
	UploadedBy string    `json:"uploaded_by,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// ListAssets is the request for one tenant's media library. The slug builds
// the library URLs.
type ListAssets struct {
	TenantID   string
	TenantSlug string
}

// ListAssetsHandler handles the ListAssets query.
type ListAssetsHandler struct {
	assets  domain.MediaRepository
	blobs   domain.BlobStore
	baseURL string
}

// NewListAssetsHandler builds the handler, failing fast on a nil dependency.
func NewListAssetsHandler(assets domain.MediaRepository, blobs domain.BlobStore,
	baseURL string) ListAssetsHandler {
	if assets == nil || blobs == nil {
		panic("nil dependency")
	}
	return ListAssetsHandler{assets: assets, blobs: blobs, baseURL: baseURL}
}

// Handle returns the tenant's media assets, newest first, each with a fresh
// preview URL.
func (h ListAssetsHandler) Handle(ctx context.Context, q ListAssets) ([]AssetView, error) {
	rows, err := h.assets.List(ctx, q.TenantID)
	if err != nil {
		return nil, err
	}
	out := make([]AssetView, 0, len(rows))
	for _, a := range rows {
		preview, err := h.blobs.PresignGet(ctx, a.StorageKey(), PreviewTTL, a.Disposition())
		if err != nil {
			return nil, err
		}
		out = append(out, AssetView{
			ID: a.ID(), Filename: a.Filename(), ContentType: a.ContentType(),
			SizeBytes: a.SizeBytes(), URL: domain.LibraryURL(h.baseURL, q.TenantSlug, a.ID()),
			PreviewURL: preview, Width: a.Width(), Height: a.Height(),
			Published: a.IsPublished(), UploadedBy: a.UploadedBy(), CreatedAt: a.CreatedAt(),
		})
	}
	return out, nil
}
```

Create `internal/media/app/query/asset_content.go`:

```go
package query

import (
	"context"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/media/domain"
)

// ContentURLTTL is how long the URL behind a library URL's redirect stays
// valid. The API caches the redirect for 10 minutes, so a cached redirect
// never points at an expired signature.
const ContentURLTTL = 15 * time.Minute

// AssetContentURL is the request for a short-lived URL that reads one
// library asset.
type AssetContentURL struct {
	TenantID string
	AssetID  string
}

// AssetContentURLHandler handles the AssetContentURL query.
type AssetContentURLHandler struct {
	assets domain.MediaRepository
	blobs  domain.BlobStore
}

// NewAssetContentURLHandler builds the handler, failing fast on a nil dependency.
func NewAssetContentURLHandler(assets domain.MediaRepository, blobs domain.BlobStore) AssetContentURLHandler {
	if assets == nil || blobs == nil {
		panic("nil dependency")
	}
	return AssetContentURLHandler{assets: assets, blobs: blobs}
}

// Handle loads the asset under RLS (another tenant's id is not found) and
// presigns a read of its private object.
func (h AssetContentURLHandler) Handle(ctx context.Context, q AssetContentURL) (string, error) {
	a, err := h.assets.Get(ctx, q.TenantID, q.AssetID)
	if err != nil {
		return "", err
	}
	return h.blobs.PresignGet(ctx, a.StorageKey(), ContentURLTTL, a.Disposition())
}
```

In `internal/media/app/application.go`, add to `Queries`:

```go
	AssetContentURL decorator.QueryHandler[query.AssetContentURL, string]
```

- [ ] **Step 7: Update the command tests**

In `internal/media/app/command/upload_asset_test.go`:

1. Add `"encoding/base64"`, `"image"`, `"image/png"`, and `"time"` to the imports.
2. Add a `MarkPublished` method to `fakeAssets`, plus these helpers:

```go
func (f *fakeAssets) MarkPublished(_ context.Context, tenantID, id string, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, a := range f.items {
		if a.TenantID() == tenantID && a.ID() == id {
			if !a.IsPublished() {
				f.items[i] = domain.HydrateMediaAsset(a.ID(), a.TenantID(), a.Filename(),
					a.ContentType(), a.SizeBytes(), a.Width(), a.Height(), a.StorageKey(),
					a.UploadedBy(), &at, a.CreatedAt())
			}
			return nil
		}
	}
	return domain.ErrMediaNotFound
}

const webpAlpha1x1 = "UklGRkoAAABXRUJQVlA4WAoAAAAQAAAAAAAAAAAAQUxQSAwAAAARBxAR/Q9ERP8DAABWUDggGAAAABQBAJ0BKgEAAQAAAP4AAA3AAP7mtQAAAA=="

// pngBytes encodes a transparent w×h PNG.
func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, w, h))))
	return buf.Bytes()
}

func newUploader(repo *fakeAssets, blobs *adapters.MemoryBlobStore, maxBytes int64) command.UploadAssetHandler {
	return command.NewUploadAssetHandler(repo, blobs, maxBytes, "https://app.test")
}

// uploadPNG uploads a 4×4 PNG named name for tenant-1 (slug acme).
func uploadPNG(t *testing.T, repo *fakeAssets, blobs *adapters.MemoryBlobStore, name string) command.UploadAssetResult {
	t.Helper()
	res, err := newUploader(repo, blobs, 1<<20).Handle(context.Background(), command.UploadAsset{
		TenantID: "tenant-1", TenantSlug: "acme", Filename: name,
		Body: bytes.NewReader(pngBytes(t, 4, 4)),
	})
	require.NoError(t, err)
	return res
}
```

3. Replace every test from `TestUploadAsset_persistsAfterBytesWritten` to the end of the file:

```go
func TestUploadAsset_persistsAfterBytesWritten(t *testing.T) {
	repo := &fakeAssets{}
	blobs := adapters.NewMemoryBlobStore("https://media.test")

	result, err := newUploader(repo, blobs, 1<<20).Handle(context.Background(), command.UploadAsset{
		TenantID: "tenant-1", TenantSlug: "acme", Filename: "logo.png",
		Body: bytes.NewReader(pngBytes(t, 10, 10)), UploadedBy: "user-1",
	})
	require.NoError(t, err)
	require.Equal(t, "https://app.test/t/acme/api/media/"+result.AssetID+"/content", result.URL)
	require.Equal(t, 10, result.Width)

	_, ct, ok := blobs.Get("media/tenant-1/" + result.AssetID + "/logo.png")
	require.True(t, ok, "object must be written")
	require.Equal(t, "image/png", ct)
	require.False(t, blobs.IsPublic("media/tenant-1/"+result.AssetID+"/logo.png"))

	list, err := query.NewListAssetsHandler(repo, blobs, "https://app.test").Handle(context.Background(),
		query.ListAssets{TenantID: "tenant-1", TenantSlug: "acme"})
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, result.URL, list[0].URL)
	require.Equal(t, "https://media.test/media/tenant-1/"+result.AssetID+"/logo.png?signature=test",
		list[0].PreviewURL)
	require.Equal(t, 10, list[0].Width)
	require.False(t, list[0].Published)
}

func TestUploadAsset_detectsTypeFromBytes(t *testing.T) {
	for name, body := range map[string][]byte{
		"evil.png": []byte("MZ\x90\x00 an executable"),
		"logo.svg": []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
	} {
		repo := &fakeAssets{}
		blobs := adapters.NewMemoryBlobStore("https://media.test")
		_, err := newUploader(repo, blobs, 1<<20).Handle(context.Background(), command.UploadAsset{
			TenantID: "tenant-1", TenantSlug: "acme", Filename: name, Body: bytes.NewReader(body),
		})
		require.ErrorIs(t, err, domain.ErrUnsupportedMediaType, name)
		require.Zero(t, blobs.Len(), "no bytes are written when the type is rejected")
		require.Empty(t, repo.items)
	}
}

func TestUploadAsset_convertsWebP(t *testing.T) {
	repo := &fakeAssets{}
	blobs := adapters.NewMemoryBlobStore("https://media.test")
	webp, err := base64.StdEncoding.DecodeString(webpAlpha1x1)
	require.NoError(t, err)

	result, err := newUploader(repo, blobs, 1<<20).Handle(context.Background(), command.UploadAsset{
		TenantID: "tenant-1", TenantSlug: "acme", Filename: "photo.webp", Body: bytes.NewReader(webp),
	})
	require.NoError(t, err)
	require.Equal(t, "photo.png", result.Filename)
	_, ct, ok := blobs.Get("media/tenant-1/" + result.AssetID + "/photo.png")
	require.True(t, ok)
	require.Equal(t, "image/png", ct)
}

func TestUploadAsset_rejectsOversize(t *testing.T) {
	repo := &fakeAssets{}
	blobs := adapters.NewMemoryBlobStore("https://media.test")
	_, err := newUploader(repo, blobs, 4).Handle(context.Background(), command.UploadAsset{
		TenantID: "tenant-1", TenantSlug: "acme", Filename: "big.png",
		Body: bytes.NewReader(bytes.Repeat([]byte("x"), 8)),
	})
	require.ErrorIs(t, err, domain.ErrMediaTooLarge)
	require.Zero(t, blobs.Len())
}

func TestUploadAsset_rejectsEmpty(t *testing.T) {
	repo := &fakeAssets{}
	blobs := adapters.NewMemoryBlobStore("https://media.test")
	_, err := newUploader(repo, blobs, 1<<20).Handle(context.Background(), command.UploadAsset{
		TenantID: "tenant-1", TenantSlug: "acme", Filename: "empty.png", Body: bytes.NewReader(nil),
	})
	require.ErrorIs(t, err, domain.ErrEmptyUpload)
	require.Zero(t, blobs.Len())
}

func TestUploadAsset_metadataFailureCompensatesBlob(t *testing.T) {
	repo := &fakeAssets{AddErr: errors.New("database is sad")}
	blobs := adapters.NewMemoryBlobStore("https://media.test")
	_, err := newUploader(repo, blobs, 1<<20).Handle(context.Background(), command.UploadAsset{
		TenantID: "tenant-1", TenantSlug: "acme", Filename: "logo.png",
		Body: bytes.NewReader(pngBytes(t, 4, 4)),
	})
	require.Error(t, err)
	require.Zero(t, blobs.Len(), "an orphaned object must be cleaned up on metadata failure")
}

func TestAssetContentURL_presignsTheLibraryObject(t *testing.T) {
	repo := &fakeAssets{}
	blobs := adapters.NewMemoryBlobStore("https://media.test")
	logo := uploadPNG(t, repo, blobs, "logo.png")

	got, err := query.NewAssetContentURLHandler(repo, blobs).Handle(context.Background(),
		query.AssetContentURL{TenantID: "tenant-1", AssetID: logo.AssetID})
	require.NoError(t, err)
	require.Equal(t, "https://media.test/media/tenant-1/"+logo.AssetID+"/logo.png?signature=test", got)

	_, err = query.NewAssetContentURLHandler(repo, blobs).Handle(context.Background(),
		query.AssetContentURL{TenantID: "tenant-2", AssetID: logo.AssetID})
	require.ErrorIs(t, err, domain.ErrMediaNotFound)
}

func TestDeleteAsset_removesRowAndObject(t *testing.T) {
	repo := &fakeAssets{}
	blobs := adapters.NewMemoryBlobStore("https://media.test")
	logo := uploadPNG(t, repo, blobs, "logo.png")

	require.NoError(t, command.NewDeleteAssetHandler(repo, blobs).Handle(context.Background(),
		command.DeleteAsset{TenantID: "tenant-1", AssetID: logo.AssetID}))
	require.Zero(t, blobs.Len())
	_, err := repo.Get(context.Background(), "tenant-1", logo.AssetID)
	require.ErrorIs(t, err, domain.ErrMediaNotFound)
}

func TestDeleteAsset_unknownIsNotFound(t *testing.T) {
	repo := &fakeAssets{}
	blobs := adapters.NewMemoryBlobStore("https://media.test")
	err := command.NewDeleteAssetHandler(repo, blobs).Handle(context.Background(),
		command.DeleteAsset{TenantID: "tenant-1", AssetID: "nope"})
	require.ErrorIs(t, err, domain.ErrMediaNotFound)
}
```

Run: `go test ./internal/media/...` (no Docker needed for these packages except adapters).
Expected: PASS for `domain`, `app/command`. If `adapters` runs, it needs `DOCKER_HOST`.

- [ ] **Step 8: Wire the service and the HTTP surface**

In `internal/service/application.go`, `buildMedia`:

```go
			UploadAsset: decorator.ApplyResultCommandDecorators(
				mediacommand.NewUploadAssetHandler(assets, blobs, cfg.MediaMaxBytes, cfg.BaseURL),
				"UploadAsset", logger),
```

```go
		Queries: mediaapp.Queries{
			ListAssets: decorator.ApplyQueryDecorators(
				mediaquery.NewListAssetsHandler(assets, blobs, cfg.BaseURL), "ListAssets", logger),
			AssetContentURL: decorator.ApplyQueryDecorators(
				mediaquery.NewAssetContentURLHandler(assets, blobs), "AssetContentURL", logger),
		},
```

In `internal/api/server.go`, after `r.Get("/media", s.handleListMedia)`:

```go
			r.Get("/media/{id}/content", s.handleMediaContent)
```

In `internal/api/media_handlers.go`:

- `handleListMedia`: pass `mediaquery.ListAssets{TenantID: ws.ID, TenantSlug: ws.Slug}`.
- `handleUploadMedia`: drop the `contentType` line, call the command as shown below, and respond with the new fields.

```go
	result, err := s.media.Commands.UploadAsset.Handle(r.Context(), mediacommand.UploadAsset{
		TenantID:   ws.ID,
		TenantSlug: ws.Slug,
		Filename:   header.Filename,
		Body:       file,
		UploadedBy: principal.ActorID(),
	})
	if err != nil {
		s.fail(w, r, "upload media", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":       result.AssetID,
		"url":      result.URL,
		"filename": result.Filename,
		"width":    result.Width,
		"height":   result.Height,
	})
```

- Add the handler:

```go
// handleMediaContent redirects a member to a short-lived presigned read of
// one library asset. Library URLs in drafts point here, so the editor
// canvas, the media picker, and a browser tab show private assets without
// making them public.
func (s *Server) handleMediaContent(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermMediaGet); !ok {
		return
	}
	target, err := s.media.Queries.AssetContentURL.Handle(r.Context(), mediaquery.AssetContentURL{
		TenantID: ws.ID, AssetID: chi.URLParam(r, "id"),
	})
	if err != nil {
		s.fail(w, r, "media content", err)
		return
	}
	// Cached for 10 minutes, under the 15-minute signature (ContentURLTTL).
	w.Header().Set("Cache-Control", "private, max-age=600")
	http.Redirect(w, r, target, http.StatusFound)
}
```

- [ ] **Step 9: Write the API test**

Create `internal/api/media_handlers_test.go`:

```go
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
```

- [ ] **Step 10: Run the whole Go suite**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock"; go build ./... && go test ./internal/media/... ./internal/api/ ./test/ -run 'Media|Asset|Upload|Migrat|S3'`
Expected: PASS. The migration test now expects version 24. Then run `go test ./...` once and expect PASS; for the known flaky tests, follow Global Constraints.

- [ ] **Step 11: Commit**

```bash
git add internal/db/migrations/000024_private_media.up.sql internal/db/migrations/000024_private_media.down.sql \
  test/migrate_test.go internal/media internal/service/application.go \
  internal/api/media_handlers.go internal/api/server.go internal/api/media_handlers_test.go
git commit -m "feat(media): keep the library private and serve members through presigned redirects"
```

---

### Task 5: PublishAssets command

**Files:**
- Create: `internal/media/app/command/publish_assets.go`
- Create: `internal/media/app/command/publish_assets_test.go`
- Modify: `internal/media/domain/errors.go`
- Modify: `internal/media/app/application.go`
- Modify: `internal/service/application.go` (`buildMedia`)

**Interfaces:**
- Consumes: from Task 2, `NewLibraryURLs`; from Task 3, `Publish`, `PublishedKey`, and `PublicURL`; from Task 4, `MarkPublished`, `IsPublished`, `Disposition`, and the test helpers `fakeAssets`, `uploadPNG`, and `pngBytes` (same `command_test` package).
- Produces:
  - `command.PublishAssets{TenantID string; Texts []string}`.
  - `command.PublishAssetsResult{Texts []string}`, with texts in input order.
  - `command.NewPublishAssetsHandler(assets, blobs, baseURL string)`.
  - `mediaapp.Commands.PublishAssets` of type `decorator.ResultCommandHandler[command.PublishAssets, command.PublishAssetsResult]`.
  - `domain.ErrMediaMissing` (slug `media_missing`). Its message lists the missing asset ids.

- [ ] **Step 1: Write the failing tests**

Create `internal/media/app/command/publish_assets_test.go`:

```go
package command_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/media/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/media/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/media/domain"
)

func TestPublishAssets_copiesOnceAndRewrites(t *testing.T) {
	ctx := context.Background()
	repo := &fakeAssets{}
	blobs := adapters.NewMemoryBlobStore("https://media.test")
	logo := uploadPNG(t, repo, blobs, "logo.png")
	publish := command.NewPublishAssetsHandler(repo, blobs, "https://app.test")

	html := `<img src="` + logo.URL + `"><td style="background:url(` + logo.URL + `)">`
	text := "Logo: " + logo.URL
	res, err := publish.Handle(ctx, command.PublishAssets{TenantID: "tenant-1", Texts: []string{html, text}})
	require.NoError(t, err)

	key := "published/tenant-1/" + logo.AssetID + "/logo.png"
	public := "https://media.test/" + key
	require.Equal(t, []string{
		`<img src="` + public + `"><td style="background:url(` + public + `)">`,
		"Logo: " + public,
	}, res.Texts)
	_, ct, ok := blobs.Get(key)
	require.True(t, ok)
	require.Equal(t, "image/png", ct)
	require.True(t, blobs.IsPublic(key))
	require.Equal(t, "inline", blobs.DispositionOf(key))
	asset, err := repo.Get(ctx, "tenant-1", logo.AssetID)
	require.NoError(t, err)
	require.True(t, asset.IsPublished())

	// A published asset is not copied again.
	blobs.PublishErr = errors.New("must not copy twice")
	again, err := publish.Handle(ctx, command.PublishAssets{TenantID: "tenant-1", Texts: []string{html}})
	require.NoError(t, err)
	require.Equal(t, res.Texts[0], again.Texts[0])
}

func TestPublishAssets_missingAssetPublishesNothing(t *testing.T) {
	ctx := context.Background()
	repo := &fakeAssets{}
	blobs := adapters.NewMemoryBlobStore("https://media.test")
	logo := uploadPNG(t, repo, blobs, "logo.png")
	ghost := uuid.NewString()
	html := `<img src="` + logo.URL + `"><img src="` +
		domain.LibraryURL("https://app.test", "acme", ghost) + `">`

	_, err := command.NewPublishAssetsHandler(repo, blobs, "https://app.test").Handle(ctx,
		command.PublishAssets{TenantID: "tenant-1", Texts: []string{html}})
	require.ErrorIs(t, err, domain.ErrMediaMissing)
	require.ErrorContains(t, err, ghost)
	require.Equal(t, 1, blobs.Len(), "only the library object exists; nothing was published")
}

func TestPublishAssets_foreignTenantAssetIsMissing(t *testing.T) {
	repo := &fakeAssets{}
	blobs := adapters.NewMemoryBlobStore("https://media.test")
	logo := uploadPNG(t, repo, blobs, "logo.png") // owned by tenant-1

	_, err := command.NewPublishAssetsHandler(repo, blobs, "https://app.test").Handle(context.Background(),
		command.PublishAssets{TenantID: "tenant-2", Texts: []string{`<img src="` + logo.URL + `">`}})
	require.ErrorIs(t, err, domain.ErrMediaMissing)
}

func TestPublishAssets_passesThroughTextWithoutLibraryURLs(t *testing.T) {
	repo := &fakeAssets{}
	blobs := adapters.NewMemoryBlobStore("https://media.test")
	in := []string{`<img src="https://cdn.example/x.png">`, ""}
	res, err := command.NewPublishAssetsHandler(repo, blobs, "https://app.test").Handle(context.Background(),
		command.PublishAssets{TenantID: "tenant-1", Texts: in})
	require.NoError(t, err)
	require.Equal(t, in, res.Texts)
}
```

Run: `go test ./internal/media/app/command/ -run PublishAssets`
Expected: FAIL to compile with `undefined: command.NewPublishAssetsHandler`.

- [ ] **Step 2: Implement**

Add to the `var (...)` block in `internal/media/domain/errors.go`:

```go
	// ErrMediaMissing is returned when content that is going live references
	// a library asset that no longer exists (or belongs to another tenant).
	// Nothing is published or sent.
	ErrMediaMissing = apperr.NewIncorrectInput("media_missing",
		"a referenced media asset no longer exists")
```

Create `internal/media/app/command/publish_assets.go`:

```go
package command

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/media/domain"
)

// PublishAssets is the request to make the library assets that texts
// reference publicly reachable and to rewrite their library URLs to the
// public copies. Callers run it when content goes live: a campaign start, a
// transactional template save, a branding save.
type PublishAssets struct {
	TenantID string
	Texts    []string
}

// PublishAssetsResult carries the rewritten texts, in input order.
type PublishAssetsResult struct {
	Texts []string
}

// PublishAssetsHandler handles the PublishAssets command.
type PublishAssetsHandler struct {
	assets domain.MediaRepository
	blobs  domain.BlobStore
	urls   domain.LibraryURLs
}

// NewPublishAssetsHandler builds the handler, failing fast on a nil
// dependency. baseURL is the app origin library URLs use.
func NewPublishAssetsHandler(assets domain.MediaRepository, blobs domain.BlobStore,
	baseURL string) PublishAssetsHandler {
	if assets == nil || blobs == nil {
		panic("nil dependency")
	}
	return PublishAssetsHandler{assets: assets, blobs: blobs, urls: domain.NewLibraryURLs(baseURL)}
}

// Handle resolves every referenced asset under RLS first and fails with
// ErrMediaMissing, copying nothing, if any is gone. Then it copies each
// not-yet-published asset to its public key, marks it published, and
// rewrites the texts.
func (h PublishAssetsHandler) Handle(ctx context.Context, cmd PublishAssets) (PublishAssetsResult, error) {
	ids := h.urls.AssetIDs(cmd.Texts...)
	if len(ids) == 0 {
		return PublishAssetsResult{Texts: cmd.Texts}, nil
	}
	assets := make(map[string]*domain.MediaAsset, len(ids))
	var missing []string
	for _, id := range ids {
		a, err := h.assets.Get(ctx, cmd.TenantID, id)
		if errors.Is(err, domain.ErrMediaNotFound) {
			missing = append(missing, id)
			continue
		}
		if err != nil {
			return PublishAssetsResult{}, err
		}
		assets[id] = a
	}
	if len(missing) > 0 {
		return PublishAssetsResult{}, domain.ErrMediaMissing.WithMessage(
			"these media assets were deleted from the library: " + strings.Join(missing, ", "))
	}

	for _, id := range ids {
		a := assets[id]
		if a.IsPublished() {
			continue
		}
		if err := h.blobs.Publish(ctx, a.StorageKey(), h.publishedKey(a),
			a.ContentType(), a.Disposition()); err != nil {
			return PublishAssetsResult{}, err
		}
		if err := h.assets.MarkPublished(ctx, cmd.TenantID, id, time.Now()); err != nil {
			return PublishAssetsResult{}, err
		}
	}

	out := make([]string, len(cmd.Texts))
	for i, t := range cmd.Texts {
		out[i] = h.urls.Replace(t, func(id string) string {
			return h.blobs.PublicURL(h.publishedKey(assets[id]))
		})
	}
	return PublishAssetsResult{Texts: out}, nil
}

func (h PublishAssetsHandler) publishedKey(a *domain.MediaAsset) string {
	return h.blobs.PublishedKey(a.TenantID(), a.ID(), a.Filename())
}
```

In `internal/media/app/application.go`, add to `Commands`:

```go
	PublishAssets decorator.ResultCommandHandler[command.PublishAssets, command.PublishAssetsResult]
```

In `internal/service/application.go`, `buildMedia` → `Commands`:

```go
			PublishAssets: decorator.ApplyResultCommandDecorators(
				mediacommand.NewPublishAssetsHandler(assets, blobs, cfg.BaseURL),
				"PublishAssets", logger),
```

- [ ] **Step 3: Run the tests**

Run: `go build ./... && go test ./internal/media/app/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/media internal/service/application.go
git commit -m "feat(media): publish referenced assets and rewrite their library URLs"
```

---

### Task 6: Campaign start publishes media (bridge, wiring, media-ref check)

**Files:**
- Create: `internal/campaign/app/command/media_publisher.go`
- Modify: `internal/campaign/app/command/campaigns.go` (`StartCampaignHandler`)
- Modify: `internal/campaign/domain/campaign.go` (add `PublishBody`)
- Modify: `internal/campaign/domain/campaign_test.go`
- Modify: `internal/campaign/app/command/fakes_test.go`, `internal/campaign/app/command/campaigns_test.go`
- Modify: `internal/service/bridges.go`, `internal/service/application.go`
- Modify: `internal/api/media_handlers_test.go`

**Interfaces:**
- Consumes: from Task 5, `mediaapp.Commands.PublishAssets` and `domain.ErrMediaMissing`; from Task 2, `mediadomain.NewLibraryURLs`; from Task 4, the API test helpers.
- Produces:
  - `command.MediaPublisher` interface: `Publish(ctx context.Context, tenantID string, texts ...string) ([]string, error)`.
  - `command.NewStartCampaignHandler(campaigns, domains, enqueuer, media MediaPublisher)`.
  - `(*domain.Campaign).PublishBody(bodyHTML, bodyText string) error`.
  - `service.NewMediaPublisher(...) *mediaPublisher`, which satisfies both the campaign and tenant `MediaPublisher` ports.
  - `buildCampaign(pool, cfg, logger, ov, media campaigncommand.MediaPublisher)`.
  - The test fake `*fakeMediaPublisher{from, to string; err error; calls int}` in `command_test`.
  - The API test helper `(*testServer).startCampaignWithLogo() (base, assetID, campaignID string)`.

- [ ] **Step 1: Write the failing domain test**

Append to `internal/campaign/domain/campaign_test.go`:

```go
func TestCampaignPublishBodyOnlyOnDraft(t *testing.T) {
	c, err := domain.NewCampaign("tenant-1", "C", "Subj", "<p>lib</p>", "lib", "Acme", "news",
		"dom-1", "", 100)
	require.NoError(t, err)
	require.NoError(t, c.PublishBody("<p>pub</p>", "pub"))
	require.Equal(t, "<p>pub</p>", c.BodyHTML())
	require.Equal(t, "pub", c.BodyText())

	require.NoError(t, c.Start(time.Now()))
	require.ErrorIs(t, c.PublishBody("<p>x</p>", "x"), domain.ErrCampaignNotDraft)
}
```

Run: `go test ./internal/campaign/domain/ -run PublishBody`
Expected: FAIL with `c.PublishBody undefined`.

- [ ] **Step 2: Implement `PublishBody`**

Add to `internal/campaign/domain/campaign.go`, right above `Start`:

```go
// PublishBody replaces a draft's bodies with their publish-on-send form —
// media-library URLs rewritten to public copies. It runs right before Start,
// in the same transaction, so the body that goes out is exactly the one
// whose media was published.
func (c *Campaign) PublishBody(bodyHTML, bodyText string) error {
	if c.status != CampaignDraft {
		return ErrCampaignNotDraft
	}
	if err := checkBodySize(bodyHTML, bodyText); err != nil {
		return err
	}
	c.bodyHTML, c.bodyText = bodyHTML, bodyText
	return nil
}
```

Run: `go test ./internal/campaign/domain/`
Expected: PASS.

- [ ] **Step 3: Write the failing command tests**

Append to `internal/campaign/app/command/fakes_test.go` (add `"strings"` to its imports):

```go
// fakeMediaPublisher rewrites from → to in every text, standing in for the
// media context's publish-on-send. A zero value passes texts through.
type fakeMediaPublisher struct {
	from, to string
	err      error
	calls    int
}

func (p *fakeMediaPublisher) Publish(_ context.Context, _ string, texts ...string) ([]string, error) {
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	out := make([]string, len(texts))
	for i, t := range texts {
		out[i] = t
		if p.from != "" {
			out[i] = strings.ReplaceAll(t, p.from, p.to)
		}
	}
	return out, nil
}
```

In `internal/campaign/app/command/campaigns_test.go`, append `&fakeMediaPublisher{}` as the last argument to every existing `command.NewStartCampaignHandler(...)` call. There are six (`grep -n NewStartCampaignHandler internal/campaign/app/command/campaigns_test.go`). Then append these tests (add `"errors"` to the imports if absent):

```go
func TestStartCampaignPublishesMedia(t *testing.T) {
	t.Parallel()
	campaigns := newFakeCampaignRepo()
	enq := &fakeCampaignEnqueuer{}
	c, err := domain.NewCampaign("tenant-1", "C", "Subj", `<img src="LIB">`, "see LIB",
		"Acme", "news", "dom-1", "", 100)
	require.NoError(t, err)
	id, err := campaigns.Add(context.Background(), "tenant-1", c)
	require.NoError(t, err)
	campaigns.targets[id] = []domain.Target{{ListID: "list-1"}}

	media := &fakeMediaPublisher{from: "LIB", to: "PUB"}
	h := command.NewStartCampaignHandler(campaigns, fakeDomainLookup{verified: true}, enq, media)
	require.NoError(t, h.Handle(context.Background(), command.StartCampaign{
		TenantID: "tenant-1", CampaignID: id,
	}))

	stored := campaigns.byID[id]
	require.Equal(t, `<img src="PUB">`, stored.BodyHTML())
	require.Equal(t, "see PUB", stored.BodyText())
	require.Equal(t, domain.CampaignRunning, stored.Status())
}

func TestStartCampaignWithMissingMediaStaysDraft(t *testing.T) {
	t.Parallel()
	campaigns := newFakeCampaignRepo()
	enq := &fakeCampaignEnqueuer{}
	id := addDraftCampaign(t, campaigns, "dom-1", []domain.Target{{ListID: "list-1"}})
	missing := errors.New("media missing")

	h := command.NewStartCampaignHandler(campaigns, fakeDomainLookup{verified: true}, enq,
		&fakeMediaPublisher{err: missing})
	err := h.Handle(context.Background(), command.StartCampaign{TenantID: "tenant-1", CampaignID: id})
	require.ErrorIs(t, err, missing)
	require.Equal(t, domain.CampaignDraft, campaigns.byID[id].Status())
	require.Empty(t, enq.starts)
}
```

Run: `go test ./internal/campaign/app/command/ -run StartCampaign`
Expected: FAIL to compile (too many arguments to `NewStartCampaignHandler`).

- [ ] **Step 4: Implement the port and the start step**

Create `internal/campaign/app/command/media_publisher.go`:

```go
package command

import "context"

// MediaPublisher makes the media-library assets that content references
// publicly reachable and returns the content with their library URLs
// rewritten to the public copies, in the order given. It fails with the
// media context's media_missing error, publishing nothing, when a referenced
// asset no longer exists. Wired in internal/service over the media context.
type MediaPublisher interface {
	Publish(ctx context.Context, tenantID string, texts ...string) ([]string, error)
}
```

In `internal/campaign/app/command/campaigns.go`:

- Add the field `media MediaPublisher` to `StartCampaignHandler`.
- Change the constructor:

```go
// NewStartCampaignHandler builds the handler, failing fast on a nil dependency.
func NewStartCampaignHandler(campaigns domain.CampaignRepository,
	domains domain.SendingDomainLookup, enqueuer CampaignEnqueuer,
	media MediaPublisher) StartCampaignHandler {
	if campaigns == nil || domains == nil || enqueuer == nil || media == nil {
		panic("nil dependency")
	}
	return StartCampaignHandler{campaigns: campaigns, domains: domains, enqueuer: enqueuer, media: media}
}
```

- Replace the `Update` closure in `Handle`:

```go
	if err := h.campaigns.Update(ctx, cmd.TenantID, cmd.CampaignID,
		func(c *domain.Campaign) (*domain.Campaign, error) {
			// Publish the body that is locked here, so a concurrent draft edit
			// cannot slip in between publishing and starting.
			// ponytail: S3 copies run inside this transaction (a few hundred ms
			// per start); split into publish-then-verify if starts ever contend.
			published, err := h.media.Publish(ctx, cmd.TenantID, c.BodyHTML(), c.BodyText())
			if err != nil {
				return nil, err
			}
			if err := c.PublishBody(published[0], published[1]); err != nil {
				return nil, err
			}
			return c, c.Start(time.Now())
		}); err != nil {
		return err
	}
```

- Update the doc comment on `Handle` to mention the publish step: "…then publishes the media its body references, transitions it to running, and enqueues the send."

- [ ] **Step 5: Bridge and wire**

Append to `internal/service/bridges.go`, adding imports for `mediacommand "github.com/nikolaymatrosov/nvelope/internal/media/app/command"` and `"github.com/nikolaymatrosov/nvelope/internal/platform/decorator"` if absent:

```go
// mediaPublisher adapts the media context's PublishAssets command to the
// MediaPublisher ports the campaign and tenant contexts declare.
type mediaPublisher struct {
	publish decorator.ResultCommandHandler[mediacommand.PublishAssets, mediacommand.PublishAssetsResult]
}

// NewMediaPublisher builds the bridge over the media context's publish command.
func NewMediaPublisher(publish decorator.ResultCommandHandler[mediacommand.PublishAssets,
	mediacommand.PublishAssetsResult]) *mediaPublisher {
	return &mediaPublisher{publish: publish}
}

// Publish publishes the assets texts reference and returns the rewritten texts.
func (p *mediaPublisher) Publish(ctx context.Context, tenantID string, texts ...string) ([]string, error) {
	res, err := p.publish.Handle(ctx, mediacommand.PublishAssets{TenantID: tenantID, Texts: texts})
	if err != nil {
		return nil, err
	}
	return res.Texts, nil
}
```

In `internal/service/application.go`:

1. In `NewApplication`, move `media := buildMedia(pool, cfg, logger, ov)` to just above `tenant := tenantapp.Application{`. Below it add `publisher := NewMediaPublisher(media.Commands.PublishAssets)`, and delete the old `media := …` line further down.
2. Change the campaign call to `campaign, tracking := buildCampaign(pool, cfg, logger, ov, publisher)`.
3. Change the `buildCampaign` signature to `func buildCampaign(pool *pgxpool.Pool, cfg config.Config, logger *slog.Logger, ov overrides, media campaigncommand.MediaPublisher) (`.
4. Change `NewStartCampaignHandler(campaigns, lookup, enqueuer)` to `NewStartCampaignHandler(campaigns, lookup, enqueuer, media)`.
5. Replace `tenantMediaRefValidator` (type and method) with:

```go
// libraryMediaRefs answers the FR-021 media-ref check: an image must
// reference a media-library URL under this deployment's BASE_URL. The tenant
// itself is checked by the BFF (slug in the URL) and, finally, by RLS when
// the asset is published at send time — Go's HTTP save path does not decode
// VisualDoc nodes, so a slug check here would only reach the converter.
type libraryMediaRefs struct{ urls mediadomain.LibraryURLs }

func (v libraryMediaRefs) IsTenantMediaRef(ref string) bool { return v.urls.Is(ref) }
```

and the construction line with:

```go
	mediaRefs := libraryMediaRefs{urls: mediadomain.NewLibraryURLs(cfg.BaseURL)}
```

If `strings` becomes unused in `application.go`, remove the import; `go build` reports it.

Run: `go build ./... && go test ./internal/campaign/... ./internal/service/...`
Expected: PASS.

- [ ] **Step 6: Write the API tests**

Append to `internal/api/media_handlers_test.go`, adding imports `"github.com/nikolaymatrosov/nvelope/internal/dbtest"`:

```go
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
```

- [ ] **Step 7: Run the tests**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock"; go test ./internal/campaign/... ./internal/api/ -run 'Campaign|Media'`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/campaign internal/service internal/api/media_handlers_test.go
git commit -m "feat(campaign): publish referenced media when a campaign starts"
```

---

### Task 7: Transactional templates publish on save

**Files:**
- Modify: `internal/campaign/app/command/media_publisher.go` (add the helper)
- Modify: `internal/campaign/app/command/templates.go`
- Modify: `internal/campaign/app/command/save_visual_template.go`
- Modify: `internal/campaign/app/command/templates_test.go`
- Modify: `internal/service/application.go` (`buildCampaign`)

**Interfaces:**
- Consumes: from Task 6, `MediaPublisher` and `fakeMediaPublisher`.
- Produces:
  - `NewCreateTemplateHandler(templates, policy, media MediaPublisher)`.
  - `NewUpdateTemplateHandler(templates, policy, media MediaPublisher)`.
  - `NewSaveVisualTemplateHandler(templates, fields, mediaRefs, media MediaPublisher)`.

- [ ] **Step 1: Write the failing tests**

In `internal/campaign/app/command/templates_test.go`, add `&fakeMediaPublisher{}` as the last argument to the existing `NewCreateTemplateHandler` and `NewUpdateTemplateHandler` calls. Add `"errors"`, `"time"`, and `"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"` to the imports if absent. Then append:

```go
const (
	libImage = "https://lib.test/x.png"
	pubImage = "https://pub.test/x.png"
)

func TestCreateTransactionalTemplatePublishesMedia(t *testing.T) {
	t.Parallel()
	templates := newFakeTemplateRepo()
	media := &fakeMediaPublisher{from: libImage, to: pubImage}
	h := command.NewCreateTemplateHandler(templates, fakeRawHTMLPolicy{sanitize: true}, media)
	res, err := h.Handle(context.Background(), command.CreateTemplate{
		TenantID: "tenant-1", Name: "Receipt", Kind: "transactional", Subject: "Receipt",
		BodyHTML: `<p><img src="` + libImage + `"></p>`, BodyText: "logo " + libImage,
	})
	require.NoError(t, err)
	tpl, err := templates.Get(context.Background(), "tenant-1", res.TemplateID)
	require.NoError(t, err)
	require.Contains(t, tpl.BodyHTML(), pubImage)
	require.Equal(t, "logo "+pubImage, tpl.BodyText())
}

func TestCreateCampaignTemplateDoesNotPublish(t *testing.T) {
	t.Parallel()
	templates := newFakeTemplateRepo()
	media := &fakeMediaPublisher{from: libImage, to: pubImage}
	h := command.NewCreateTemplateHandler(templates, fakeRawHTMLPolicy{sanitize: true}, media)
	res, err := h.Handle(context.Background(), command.CreateTemplate{
		TenantID: "tenant-1", Name: "Newsletter", Kind: "campaign", Subject: "News",
		BodyHTML: `<p><img src="` + libImage + `"></p>`,
	})
	require.NoError(t, err)
	tpl, err := templates.Get(context.Background(), "tenant-1", res.TemplateID)
	require.NoError(t, err)
	require.Contains(t, tpl.BodyHTML(), libImage, "campaign templates stay private")
	require.Zero(t, media.calls)
}

func TestUpdateTransactionalTemplateWithMissingMediaKeepsRow(t *testing.T) {
	t.Parallel()
	templates := newFakeTemplateRepo()
	id := seedTemplate(t, templates, domain.KindTransactional)
	missing := errors.New("media missing")
	h := command.NewUpdateTemplateHandler(templates, fakeRawHTMLPolicy{sanitize: true},
		&fakeMediaPublisher{err: missing})
	_, err := h.Handle(context.Background(), command.UpdateTemplate{
		TenantID: "tenant-1", TemplateID: id, Name: "T", Subject: "S",
		BodyHTML: `<p><img src="` + libImage + `"></p>`,
	})
	require.ErrorIs(t, err, missing)
	tpl, err := templates.Get(context.Background(), "tenant-1", id)
	require.NoError(t, err)
	require.Equal(t, "<p>tpl</p>", tpl.BodyHTML())
}

func TestSaveVisualTransactionalTemplatePublishesMedia(t *testing.T) {
	t.Parallel()
	templates := newFakeTemplateRepo()
	stamp := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	templates.byID["tpl-v"] = domain.HydrateTemplate("tpl-v", "tenant-1", "Receipt",
		domain.KindTransactional, "Receipt", "<p>old</p>", "old", nil, nil, stamp, stamp)

	media := &fakeMediaPublisher{from: libImage, to: pubImage}
	h := command.NewSaveVisualTemplateHandler(templates, fakeFieldsProvider{},
		mediaPrefixValidator{prefix: "https://lib.test/"}, media)
	_, err := h.Handle(context.Background(), command.SaveVisualTemplate{
		TenantID: "tenant-1", TemplateID: "tpl-v", Subject: "Receipt",
		Doc: &domain.VisualDoc{Version: 1, Nodes: []domain.Node{
			domain.Image{MediaRef: libImage, Alt: "logo"},
		}},
		BodyHTML:          `<p><img src="` + libImage + `" alt="logo"></p>`,
		BodyText:          "logo",
		IfUnmodifiedSince: stamp,
	})
	require.NoError(t, err)
	require.Contains(t, templates.byID["tpl-v"].BodyHTML(), pubImage)
	require.Equal(t, 1, media.calls)
}
```

Run: `go test ./internal/campaign/app/command/ -run Template`
Expected: FAIL to compile (too many arguments).

- [ ] **Step 2: Implement**

Append to `internal/campaign/app/command/media_publisher.go`, adding the import `"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"`:

```go
// publishIfTransactional publishes the media a transactional template's
// bodies reference. Such a template is live once saved, because the API can
// send it at any moment. Campaign templates stay private until a campaign
// built from them starts.
func publishIfTransactional(ctx context.Context, media MediaPublisher, tenantID string,
	kind domain.Kind, bodyHTML, bodyText string) (string, string, error) {
	if kind != domain.KindTransactional {
		return bodyHTML, bodyText, nil
	}
	out, err := media.Publish(ctx, tenantID, bodyHTML, bodyText)
	if err != nil {
		return "", "", err
	}
	return out[0], out[1], nil
}
```

In `internal/campaign/app/command/templates.go`:

- Add `media MediaPublisher` to `CreateTemplateHandler` and `UpdateTemplateHandler`. Add the constructor parameter, the nil check (`|| media == nil`), and the assignment.
- `CreateTemplateHandler.Handle`: between `applyRawHTMLPolicy` and `domain.NewTemplate`:

```go
	bodyHTML, bodyText, err := publishIfTransactional(ctx, h.media, cmd.TenantID,
		domain.Kind(cmd.Kind), bodyHTML, cmd.BodyText)
	if err != nil {
		return CreateTemplateResult{}, err
	}
	tpl, err := domain.NewTemplate(cmd.TenantID, cmd.Name, domain.Kind(cmd.Kind),
		cmd.Subject, bodyHTML, bodyText)
```

(The `bodyHTML, warnings, err :=` line above stays. The new line uses `:=` because it introduces `bodyText`.)

- `UpdateTemplateHandler.Handle` closure:

```go
		func(tpl *domain.Template) (*domain.Template, error) {
			html, text, err := publishIfTransactional(ctx, h.media, cmd.TenantID, tpl.Kind(),
				bodyHTML, cmd.BodyText)
			if err != nil {
				return nil, err
			}
			return tpl, tpl.Recompose(cmd.Name, cmd.Subject, html, text)
		}); err != nil {
```

In `internal/campaign/app/command/save_visual_template.go`:

- Add `media MediaPublisher` to `SaveVisualTemplateHandler`. The constructor becomes `NewSaveVisualTemplateHandler(templates domain.TemplateRepository, fields FieldsProvider, mediaRefs MediaRefValidator, media MediaPublisher)`, with `|| media == nil` in the nil check.
- In the closure, after the stale-row check, add:

```go
			html, text, err := publishIfTransactional(ctx, h.media, cmd.TenantID, t.Kind(),
				sanitizedHTML, cmd.BodyText)
			if err != nil {
				return nil, err
			}
			if err := t.ApplyVisualSave(
				cmd.Subject, cmd.Doc, cmd.PinnedTheme,
				html, text,
				cmd.DocJSON, cmd.ThemeJSON, warnings,
				slugSetFieldSet(slugs), h.mediaRefs,
			); err != nil {
```

In `internal/service/application.go` (`buildCampaign`), pass `media` as the last argument to `NewCreateTemplateHandler`, `NewUpdateTemplateHandler`, and `NewSaveVisualTemplateHandler`.

- [ ] **Step 3: Run the tests**

Run: `go build ./... && go test ./internal/campaign/...`
Expected: PASS. Other callers of the three constructors, if any, fail to compile; add `&fakeMediaPublisher{}` there too. Find them with `grep -rn "NewCreateTemplateHandler\|NewUpdateTemplateHandler\|NewSaveVisualTemplateHandler" internal`.

- [ ] **Step 4: Commit**

```bash
git add internal/campaign internal/service/application.go
git commit -m "feat(campaign): publish a transactional template's media when it is saved"
```

---

### Task 8: Branding logo from the library

**Files:**
- Modify: `internal/tenant/app/command/save_branding.go`
- Create: `internal/tenant/app/command/save_branding_test.go`
- Modify: `internal/service/application.go`

**Interfaces:**
- Consumes: from Task 6, the `publisher` built in `NewApplication`.
- Produces: `tenantcommand.MediaPublisher` (same method set as the campaign port) and `NewSaveBrandingHandler(branding, media MediaPublisher)`.

- [ ] **Step 1: Write the failing tests**

First check the package name: `head -1 internal/tenant/app/command/command_test.go`. It is expected to be `package command_test`; use that package here.

Create `internal/tenant/app/command/save_branding_test.go`:

```go
package command_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/tenant/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

type fakeBrandingRepo struct{ saved *domain.TenantBranding }

func (r *fakeBrandingRepo) Get(_ context.Context, tenantID string) (*domain.TenantBranding, error) {
	return domain.HydrateTenantBranding(tenantID, "", "", "", time.Time{}), nil
}

func (r *fakeBrandingRepo) Save(_ context.Context, b *domain.TenantBranding) error {
	r.saved = b
	return nil
}

// fakeLogoPublisher rewrites from → to; an empty from passes texts through.
type fakeLogoPublisher struct {
	from, to string
	err      error
}

func (p fakeLogoPublisher) Publish(_ context.Context, _ string, texts ...string) ([]string, error) {
	if p.err != nil {
		return nil, p.err
	}
	out := make([]string, len(texts))
	for i, t := range texts {
		out[i] = t
		if p.from != "" {
			out[i] = strings.ReplaceAll(t, p.from, p.to)
		}
	}
	return out, nil
}

const (
	libraryLogo = "https://app.test/t/acme/api/media/0b9f3c3e-6a7d-4c1e-9a55-2f1d7e4b8c90/content"
	publicLogo  = "https://media.test/published/t1/0b9f3c3e-6a7d-4c1e-9a55-2f1d7e4b8c90/logo.png"
)

func TestSaveBrandingPublishesLibraryLogo(t *testing.T) {
	repo := &fakeBrandingRepo{}
	h := command.NewSaveBrandingHandler(repo, fakeLogoPublisher{from: libraryLogo, to: publicLogo})
	require.NoError(t, h.Handle(context.Background(), command.SaveBranding{
		TenantID: "t1", LogoURL: libraryLogo,
	}))
	require.Equal(t, publicLogo, repo.saved.LogoURL())
}

func TestSaveBrandingKeepsExternalLogo(t *testing.T) {
	repo := &fakeBrandingRepo{}
	h := command.NewSaveBrandingHandler(repo, fakeLogoPublisher{})
	require.NoError(t, h.Handle(context.Background(), command.SaveBranding{
		TenantID: "t1", LogoURL: "https://cdn.example/logo.svg",
	}))
	require.Equal(t, "https://cdn.example/logo.svg", repo.saved.LogoURL())
}

func TestSaveBrandingFailsWhenLogoAssetIsGone(t *testing.T) {
	repo := &fakeBrandingRepo{}
	missing := errors.New("media missing")
	h := command.NewSaveBrandingHandler(repo, fakeLogoPublisher{err: missing})
	err := h.Handle(context.Background(), command.SaveBranding{TenantID: "t1", LogoURL: libraryLogo})
	require.ErrorIs(t, err, missing)
	require.Nil(t, repo.saved)
}
```

Run: `go test ./internal/tenant/app/command/ -run SaveBranding`
Expected: FAIL to compile (too many arguments to `NewSaveBrandingHandler`).

- [ ] **Step 2: Implement**

In `internal/tenant/app/command/save_branding.go`:

```go
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
// public URL; an external URL passes through. CSS is sanitised on save so
// the stored value is always safe to render.
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
```

In `internal/service/application.go`: `tenantcommand.NewSaveBrandingHandler(branding, publisher)`.

- [ ] **Step 3: Run the tests**

Run: `go build ./... && go test ./internal/tenant/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/tenant internal/service/application.go
git commit -m "feat(tenant): publish a branding logo picked from the media library"
```

---

### Task 9: Delete with takedown

**Files:**
- Modify: `internal/media/app/command/delete_asset.go`
- Modify: `internal/media/app/command/upload_asset_test.go` (delete tests)
- Modify: `internal/media/app/application.go`, `internal/service/application.go`
- Modify: `internal/api/media_handlers.go`
- Modify: `internal/api/helpers_test.go` (expose the memory blob store)
- Modify: `internal/api/media_handlers_test.go`

**Interfaces:**
- Consumes: from Task 5, `NewPublishAssetsHandler`; from Task 6, `startCampaignWithLogo`.
- Produces:
  - `command.DeleteAsset{TenantID, AssetID string; Unpublish bool}`.
  - `command.DeleteAssetResult{Filename string; Unpublished bool}`.
  - `mediaapp.Commands.DeleteAsset` becomes a `ResultCommandHandler`.
  - HTTP: `DELETE /t/{slug}/api/media/{id}?unpublish=true`.
  - Audit action `media.unpublished`.
  - The `testServer.blobs *mediaadapters.MemoryBlobStore` field.

- [ ] **Step 1: Write the failing command tests**

In `internal/media/app/command/upload_asset_test.go`, change the two existing delete tests to use the result. The first assertion becomes `res, err := …Handle(…); require.NoError(t, err); require.False(t, res.Unpublished)`, and the second becomes `_, err := …`. Then append:

```go
func TestDeleteAsset_keepsPublicCopyByDefault(t *testing.T) {
	ctx := context.Background()
	repo := &fakeAssets{}
	blobs := adapters.NewMemoryBlobStore("https://media.test")
	logo := uploadPNG(t, repo, blobs, "logo.png")
	_, err := command.NewPublishAssetsHandler(repo, blobs, "https://app.test").Handle(ctx,
		command.PublishAssets{TenantID: "tenant-1", Texts: []string{logo.URL}})
	require.NoError(t, err)

	res, err := command.NewDeleteAssetHandler(repo, blobs).Handle(ctx,
		command.DeleteAsset{TenantID: "tenant-1", AssetID: logo.AssetID})
	require.NoError(t, err)
	require.False(t, res.Unpublished)
	_, _, ok := blobs.Get("published/tenant-1/" + logo.AssetID + "/logo.png")
	require.True(t, ok, "sent emails keep working")
	require.Equal(t, 1, blobs.Len())
}

func TestDeleteAsset_unpublishRemovesPublicCopy(t *testing.T) {
	ctx := context.Background()
	repo := &fakeAssets{}
	blobs := adapters.NewMemoryBlobStore("https://media.test")
	logo := uploadPNG(t, repo, blobs, "logo.png")
	_, err := command.NewPublishAssetsHandler(repo, blobs, "https://app.test").Handle(ctx,
		command.PublishAssets{TenantID: "tenant-1", Texts: []string{logo.URL}})
	require.NoError(t, err)

	res, err := command.NewDeleteAssetHandler(repo, blobs).Handle(ctx,
		command.DeleteAsset{TenantID: "tenant-1", AssetID: logo.AssetID, Unpublish: true})
	require.NoError(t, err)
	require.True(t, res.Unpublished)
	require.Equal(t, "logo.png", res.Filename)
	require.Zero(t, blobs.Len())
}
```

Run: `go test ./internal/media/app/command/ -run DeleteAsset`
Expected: FAIL to compile (`Handle` returns one value).

- [ ] **Step 2: Implement**

Replace everything below the imports in `internal/media/app/command/delete_asset.go`:

```go
// DeleteAsset is the request to remove one media asset.
type DeleteAsset struct {
	TenantID string
	AssetID  string
	// Unpublish also removes the asset's public copy, which breaks the image
	// in sent emails and the archive. It exists for takedown requests and
	// mistakes; by default the public copy outlives the library asset.
	Unpublish bool
}

// DeleteAssetResult reports what was removed, for the audit trail.
type DeleteAssetResult struct {
	Filename string
	// Unpublished is true when a public copy existed and was removed.
	Unpublished bool
}

// DeleteAssetHandler handles the DeleteAsset command.
type DeleteAssetHandler struct {
	assets domain.MediaRepository
	blobs  domain.BlobStore
}

// NewDeleteAssetHandler builds the handler, failing fast on a nil dependency.
func NewDeleteAssetHandler(assets domain.MediaRepository, blobs domain.BlobStore) DeleteAssetHandler {
	if assets == nil || blobs == nil {
		panic("nil dependency")
	}
	return DeleteAssetHandler{assets: assets, blobs: blobs}
}

// Handle loads the asset (so RLS confirms the tenant owns it), removes the
// public copy first when asked (so a failure leaves everything in place for
// a retry), then the metadata row, then the private object. The row is the
// source of truth: once it is gone the listing no longer shows the asset,
// even if the object delete fails and leaves an orphan in storage.
func (h DeleteAssetHandler) Handle(ctx context.Context, cmd DeleteAsset) (DeleteAssetResult, error) {
	asset, err := h.assets.Get(ctx, cmd.TenantID, cmd.AssetID)
	if err != nil {
		return DeleteAssetResult{}, err
	}
	res := DeleteAssetResult{Filename: asset.Filename()}
	if cmd.Unpublish && asset.IsPublished() {
		key := h.blobs.PublishedKey(asset.TenantID(), asset.ID(), asset.Filename())
		if err := h.blobs.Delete(ctx, key); err != nil {
			return DeleteAssetResult{}, err
		}
		res.Unpublished = true
	}
	if err := h.assets.Delete(ctx, cmd.TenantID, cmd.AssetID); err != nil {
		return DeleteAssetResult{}, err
	}
	return res, h.blobs.Delete(ctx, asset.StorageKey())
}
```

In `internal/media/app/application.go`:

```go
	DeleteAsset decorator.ResultCommandHandler[command.DeleteAsset, command.DeleteAssetResult]
```

In `internal/service/application.go`, `buildMedia`:

```go
			DeleteAsset: decorator.ApplyResultCommandDecorators(
				mediacommand.NewDeleteAssetHandler(assets, blobs), "DeleteAsset", logger),
```

Replace `handleDeleteMedia` in `internal/api/media_handlers.go`:

```go
// handleDeleteMedia removes an asset's metadata row and private object. With
// ?unpublish=true it also takes down the public copy (audited), which breaks
// the image in sent emails and the archive.
func (s *Server) handleDeleteMedia(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermMediaManage); !ok {
		return
	}
	assetID := chi.URLParam(r, "id")
	res, err := s.media.Commands.DeleteAsset.Handle(r.Context(), mediacommand.DeleteAsset{
		TenantID:  ws.ID,
		AssetID:   assetID,
		Unpublish: r.URL.Query().Get("unpublish") == "true",
	})
	if err != nil {
		s.fail(w, r, "delete media", err)
		return
	}
	if res.Unpublished {
		s.recordAudit(r.Context(), "media.unpublished", assetID,
			map[string]any{"filename": res.Filename})
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 3: Expose the blob store to API tests and add the e2e test**

In `internal/api/helpers_test.go`:

- Add a `blobs *mediaadapters.MemoryBlobStore` field to the `testServer` struct.
- In `newTestServer`, build `blobs := mediaadapters.NewMemoryBlobStore("https://media.test")` and pass it to `service.WithMediaBlobStore(blobs)` (instead of the inline constructor).
- Set `blobs: blobs` in the returned struct.

Append to `internal/api/media_handlers_test.go`:

```go
func TestMediaDeleteCanTakeDownThePublicCopy(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	base, assetID, _ := ts.startCampaignWithLogo()
	require.Equal(t, 2, ts.blobs.Len(), "the library object and its public copy")

	status, _ := ts.request(http.MethodDelete, base+"/media/"+assetID+"?unpublish=true", nil)
	require.Equal(t, http.StatusNoContent, status)
	require.Zero(t, ts.blobs.Len())

	status, body := ts.request(http.MethodGet, base+"/audit?limit=100", nil)
	require.Equal(t, http.StatusOK, status)
	var found bool
	for _, r := range body["records"].([]any) {
		if r.(map[string]any)["Action"] == "media.unpublished" {
			found = true
		}
	}
	require.True(t, found, "the takedown is audited")
}
```

- [ ] **Step 4: Run the tests**

Run: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock"; go build ./... && go test ./internal/media/... ./internal/api/ -run 'Media|Delete'`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/media internal/service/application.go internal/api
git commit -m "feat(media): let a delete also take down the published copy"
```

---

### Task 10: BFF — library URL validation and image width

**Files:**
- Modify: `frontend/src/server/validate/blocks.ts`, `frontend/src/server/validate/index.ts` (comment only)
- Modify: `frontend/src/server/validate/blocks.test.ts`
- Modify: `frontend/src/server/routes/t/[slug]/api/render-preview.post.ts`
- Modify: `frontend/src/server/routes/t/[slug]/api/campaigns/[id]/visual.put.ts`
- Modify: `frontend/src/server/routes/t/[slug]/api/templates/[id]/visual.put.ts`
- Modify: `frontend/src/server/render/types.ts`, `frontend/src/server/render/components.tsx`
- Modify: `frontend/src/server/render/render-image.test.ts`

**Interfaces:**
- Consumes: the library URL format from the Global Constraints.
- Produces:
  - `mediaLibraryPrefix(baseUrl: string | undefined, slug: string): string`, exported from `@/server/validate/blocks`.
  - `ImageBlock.attrs.width?: number | null`.

- [ ] **Step 1: Write the failing tests**

In `frontend/src/server/validate/blocks.test.ts`:

1. Change the import to `import { mediaLibraryPrefix, validateBlock, validateStyle } from "./blocks"`.
2. In `ctx()`, set `mediaUrlPrefix: "https://app.test/t/abc/api/media/",`.
3. Replace the `describe("validateBlock — image", …)` block with:

```ts
const ASSET = "0b9f3c3e-6a7d-4c1e-9a55-2f1d7e4b8c90"
const image = (attrs: Partial<{ mediaRef: string; width: number | null }>) => ({
  type: "image" as const,
  attrs: {
    mediaRef: `https://app.test/t/abc/api/media/${ASSET}/content`,
    alt: "x",
    href: "",
    ...attrs,
  },
})

describe("validateBlock — image", () => {
  it("accepts the tenant's own library URL", () => {
    expect(() => validateBlock(image({}), ctx())).not.toThrow()
  })

  it.each([
    "https://evil.test/x.png",
    `https://app.test/t/other/api/media/${ASSET}/content`,
    "https://app.test/t/abc/api/media/not-a-uuid/content",
    `https://nvelope-media.storage.yandexcloud.net/media/t1/${ASSET}/a.png`,
  ])("rejects %s", (mediaRef) => {
    expect(() => validateBlock(image({ mediaRef }), ctx())).toThrow(ValidatorError)
  })

  it("accepts a width from 1 to 600 or none", () => {
    for (const width of [1, 600, null, undefined]) {
      expect(() => validateBlock(image({ width }), ctx())).not.toThrow()
    }
  })

  it.each([0, 601, 1.5])("rejects width %s", (width) => {
    expect(() => validateBlock(image({ width }), ctx())).toThrow(ValidatorError)
  })
})

describe("mediaLibraryPrefix", () => {
  it("builds the tenant's library URL prefix", () => {
    expect(mediaLibraryPrefix("https://app.test/", "acme")).toBe("https://app.test/t/acme/api/media/")
  })

  it("is empty when the base URL is unset", () => {
    expect(mediaLibraryPrefix(undefined, "acme")).toBe("")
  })
})
```

Append to `frontend/src/server/render/render-image.test.ts`:

```ts
describe("renderVisualDoc — image width", () => {
  it("emits the width attribute and fluid CSS when the block has a width", async () => {
    const doc: VisualDoc = {
      version: 1,
      type: "doc",
      content: [
        {
          type: "image",
          attrs: { mediaRef: `${TENANT_MEDIA_PREFIX}wide.png`, alt: "Wide", href: "", width: 600 },
        },
      ],
    }
    const { bodyHtml } = await renderVisualDoc(doc, PlatformDefaultTheme)
    expect(bodyHtml).toMatch(/<img[^>]*\swidth="600"/)
    expect(bodyHtml).toMatch(/<img[^>]*max-width:100%/)
    expect(bodyHtml).toMatch(/<img[^>]*height:auto/)
  })
})
```

Run: `.claude/scripts/test-frontend.sh src/server`
Expected: FAIL (`mediaLibraryPrefix` is not exported; the width tests fail).

- [ ] **Step 2: Implement**

In `frontend/src/server/validate/blocks.ts`:

- Update the `mediaUrlPrefix` comment in `ValidateBlocksContext`: "the tenant's library URL prefix, `${NVELOPE_BASE_URL}/t/${slug}/api/media/` (see mediaLibraryPrefix). Every ImageBlock.mediaRef must be that prefix plus `<uuid>/content` (FR-021)."
- Replace `isTenantMediaRef` and add the helper:

```ts
// mediaLibraryPrefix is the start of every library URL for one tenant:
// `${NVELOPE_BASE_URL}/t/${slug}/api/media/`. It is empty when the base URL
// is unset (local dev without config), which disables the check; Go's
// publish step still refuses assets of other tenants.
export function mediaLibraryPrefix(baseUrl: string | undefined, slug: string): string {
  if (!baseUrl) return ""
  return `${baseUrl.replace(/\/+$/, "")}/t/${slug}/api/media/`
}

const LIBRARY_URL_TAIL =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\/content$/

function isTenantMediaRef(ref: string, prefix: string): boolean {
  if (!prefix) return true
  return ref.startsWith(prefix) && LIBRARY_URL_TAIL.test(ref.slice(prefix.length))
}
```

- In the `case "image":` branch, before `if (block.attrs.href) validateLink(block.attrs.href)`:

```ts
      if (block.attrs.width != null) {
        const w = block.attrs.width
        if (!Number.isInteger(w) || w < 1 || w > 600) {
          throw new ValidatorError("invalid_doc", "image width must be a whole number from 1 to 600")
        }
      }
```

In `frontend/src/server/validate/index.ts`, update the `mediaUrlPrefix` comment to "the tenant's library URL prefix — built with mediaLibraryPrefix(process.env.NVELOPE_BASE_URL, slug) in the Nitro routes."

In each of the three Nitro routes:
- Add `import { mediaLibraryPrefix } from "@/server/validate/blocks"`.
- Replace `mediaUrlPrefix: process.env.OBJECT_STORAGE_PUBLIC_BASE_URL ?? "",` with `mediaUrlPrefix: mediaLibraryPrefix(process.env.NVELOPE_BASE_URL, slug),`.

In `frontend/src/server/render/types.ts`:

```ts
export type ImageBlock = {
  type: "image"
  attrs: { mediaRef: string; alt: string; href: string; width?: number | null; style?: BlockStyle }
}
```

In `frontend/src/server/render/components.tsx`, `ImageView`:

```tsx
function ImageView({ block }: { block: ImageBlock; theme: Theme }) {
  // Image style applies to the <img> box itself (border radius / border). The
  // textAlign field, when set, positions the image within a wrapping block.
  // A width becomes the HTML attribute Outlook desktop honours (it ignores
  // CSS max-width); max-width/height:auto keep it fluid elsewhere.
  const style = block.attrs.style
  const { textAlign, ...boxStyle } = mapBlockStyle(style)
  const width = block.attrs.width ?? undefined
  const img = (
    <REImg
      src={block.attrs.mediaRef}
      alt={block.attrs.alt}
      width={width}
      style={width ? { ...boxStyle, maxWidth: "100%", height: "auto" } : boxStyle}
    />
  )
  const linked = block.attrs.href ? <RELink href={block.attrs.href}>{img}</RELink> : img
  if (textAlign) {
    return <div style={{ textAlign }}>{linked}</div>
  }
  return linked
}
```

- [ ] **Step 3: Run the tests**

Run: `.claude/scripts/test-frontend.sh src/server`
Expected: PASS, including the existing golden tests: images without `width` render as before.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/server
git commit -m "feat(bff): validate tenant library URLs and render image width for Outlook"
```

---

### Task 11: SPA — library URLs, previews, width, takedown dialog

**Files:**
- Create: `frontend/src/lib/media-urls.ts`, `frontend/src/lib/media-urls.test.ts`
- Modify: `frontend/src/lib/api-types.ts`, `frontend/src/lib/api.ts`
- Modify: `frontend/src/components/common/confirm-dialog.tsx`
- Modify: `frontend/src/components/common/media-picker.tsx`, `media-picker.test.tsx`, `media-picker.stories.tsx`
- Modify: `frontend/src/routes/t/$slug/media/index.tsx`, `index.test.tsx`, `$id.tsx`, `$id.test.tsx`
- Modify: `frontend/src/components/visual-editor/VisualEmailEditor.tsx`
- Modify: `frontend/src/components/visual-editor/extensions/ImageBlock.tsx`
- Modify: `frontend/src/components/visual-editor/plugins/imageUpload.ts`, `imageUpload.test.ts`
- Modify: `frontend/src/components/visual-editor/ui/PreviewIframe.tsx`
- Modify: `frontend/src/components/visual-editor/VisualBodyPreview.tsx`, `VisualBodyPreview.stories.tsx`
- Modify: `frontend/src/routes/t/$slug/branding/index.tsx`, `frontend/src/routes/t/$slug/campaigns/$id.tsx`, `frontend/src/routes/t/$slug/templates/$id.tsx`
- Modify: `frontend/src/locales/en/media.json`, `frontend/src/locales/ru/media.json`, `frontend/src/i18n/resources.d.ts` (regenerated)

**Interfaces:**
- Consumes: the API shapes from Tasks 4 and 9; the BFF `width` from Task 10.
- Produces:
  - `EMAIL_CONTENT_WIDTH = 600`.
  - `imageDisplayWidth(naturalWidth?: number | null): number | null`.
  - `swapLibraryUrls(html: string, assets: ReadonlyArray<{ url: string; preview_url: string }>): string`.
  - `usePreviewHtml(slug, html)`, the hook both sandboxed previews use.
  - `api.media.remove(slug, id, opts?: { unpublish?: boolean })`.
  - `applyImageBlockPick(editor, pos, mediaRef, alt, width)`.

All commands below need `export PATH="$HOME/.nvm/versions/node/v24.19.0/bin:$PATH"`.

- [ ] **Step 1: Write the failing helper tests**

Create `frontend/src/lib/media-urls.test.ts`:

```ts
import { describe, expect, it } from "vitest"
import { imageDisplayWidth, swapLibraryUrls } from "./media-urls"

describe("imageDisplayWidth", () => {
  it("caps the natural width at the email content column", () => {
    expect(imageDisplayWidth(1200)).toBe(600)
    expect(imageDisplayWidth(320)).toBe(320)
  })

  it("is null when the width is unknown (PDF)", () => {
    expect(imageDisplayWidth(undefined)).toBeNull()
    expect(imageDisplayWidth(0)).toBeNull()
  })
})

describe("swapLibraryUrls", () => {
  it("replaces every occurrence of each library URL with its preview URL", () => {
    const lib = "https://app.test/t/acme/api/media/a1/content"
    const html = `<img src="${lib}"><td style="background:url(${lib})">`
    expect(swapLibraryUrls(html, [{ url: lib, preview_url: "https://s3.test/a1?sig" }])).toBe(
      `<img src="https://s3.test/a1?sig"><td style="background:url(https://s3.test/a1?sig)">`,
    )
  })

  it("leaves URLs of unknown (deleted) assets alone", () => {
    const html = `<img src="https://app.test/t/acme/api/media/gone/content">`
    expect(swapLibraryUrls(html, [])).toBe(html)
  })
})
```

Run: `.claude/scripts/test-frontend.sh src/lib/media-urls`
Expected: FAIL (module not found).

- [ ] **Step 2: Implement the helpers and the types**

Create `frontend/src/lib/media-urls.ts`:

```ts
// Helpers for media-library URLs in the admin SPA.

// EMAIL_CONTENT_WIDTH is the email content column. Images display at most
// this wide; the library stores them at up to twice that for sharp
// rendering on high-density screens.
export const EMAIL_CONTENT_WIDTH = 600

// imageDisplayWidth is the `width` an image block gets on insert: the
// asset's natural width capped at the content column, or null when unknown.
export function imageDisplayWidth(naturalWidth?: number | null): number | null {
  if (!naturalWidth || naturalWidth <= 0) return null
  return Math.min(naturalWidth, EMAIL_CONTENT_WIDTH)
}

// swapLibraryUrls replaces each asset's library URL with its presigned
// preview URL. Preview iframes are sandboxed with an opaque origin, so the
// session cookie that library URLs need never reaches them from inside one.
export function swapLibraryUrls(
  html: string,
  assets: ReadonlyArray<{ url: string; preview_url: string }>,
): string {
  let out = html
  for (const a of assets) out = out.split(a.url).join(a.preview_url)
  return out
}

// usePreviewHtml returns html with library URLs swapped for the media
// listing's presigned URLs, for a sandboxed preview iframe. The listing is
// cached for 30 minutes, under the 1-hour signature. If the listing is
// unavailable, html is returned unchanged (images show as broken).
export function usePreviewHtml<T extends string | undefined>(slug: string, html: T): T {
  const media = useQuery({
    queryKey: queryKeys.media(slug),
    queryFn: async () => (await api.media.list(slug)).data,
    staleTime: 30 * 60 * 1000,
  })
  if (html === undefined) return html
  return swapLibraryUrls(html, media.data?.items ?? []) as T
}
```

Add these imports at the top of `media-urls.ts`:

```ts
import { useQuery } from "@tanstack/react-query"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
```

In `frontend/src/lib/api-types.ts`:

- Replace `MediaAssetView` and `MediaUploadResult`:

```ts
// MediaAssetView matches the backend's snake_case json shape.
export type MediaAssetView = {
  id: string
  filename: string
  content_type: string
  size_bytes: number
  // url is the library URL content embeds. Only workspace members can open
  // it (the API redirects to a short-lived presigned read); sending
  // rewrites it to a public copy.
  url: string
  // preview_url is a presigned read valid for an hour — for thumbnails and
  // the sandboxed preview iframe, which carries no session cookie.
  preview_url: string
  width?: number
  height?: number
  published: boolean
  uploaded_by?: string
  created_at: string
}

// Result returned by POST /media — narrower than the full view.
export type MediaUploadResult = {
  id: string
  url: string
  filename: string
  width?: number
  height?: number
}
```

- Remove `"image/svg+xml",` from `ALLOWED_MEDIA_CONTENT_TYPES`.
- In the visual-doc `ImageBlock` type (`grep -n "attrs: { mediaRef" frontend/src/lib/api-types.ts`), add `width?: number | null;` after `href: string;`.

In `frontend/src/lib/api.ts`:

```ts
    remove: (slug: string, id: string, opts: { unpublish?: boolean } = {}) =>
      request("DELETE", tp(slug, `/media/${id}${opts.unpublish ? "?unpublish=true" : ""}`)),
```

Run: `.claude/scripts/test-frontend.sh src/lib/media-urls`
Expected: PASS.

- [ ] **Step 3: Media library pages and picker**

`frontend/src/components/common/confirm-dialog.tsx`:
- Add `children?: React.ReactNode` to `ConfirmDialogProps` (import `type ReactNode` from `react` if the file does not import React types).
- Add `children` to the destructured props.
- Render `{children}` right after `</AlertDialogHeader>`.

`frontend/src/components/common/media-picker.tsx`: change the thumbnail to `src={asset.preview_url}`.

`frontend/src/routes/t/$slug/media/index.tsx`, `MediaCard`:
- Add `const [unpublish, setUnpublish] = useState(false)`.
- Change the mutation to `mutationFn: () => api.media.remove(slug, asset.id, { unpublish }),`.
- `copyUrl` writes `asset.url`; the thumbnail `src` is `asset.preview_url`; the "Open" link `href` is `asset.url`.
- Import `Checkbox` from `@/components/ui/checkbox`.
- Pass this child to the existing `ConfirmDialog`:

```tsx
      >
        {asset.published && (
          <label className="flex items-start gap-2 text-sm">
            <Checkbox
              checked={unpublish}
              onCheckedChange={(v) => setUnpublish(v === true)}
              data-testid={`media-unpublish-${asset.id}`}
            />
            <span>
              {t("deleteDialog.unpublishLabel")}
              <span className="block text-muted-foreground">
                {t("deleteDialog.unpublishWarning")}
              </span>
            </span>
          </label>
        )}
      </ConfirmDialog>
```

(i.e. `ConfirmDialog` changes from self-closing to having children.)

`frontend/src/routes/t/$slug/media/$id.tsx`:
- The preview `<img>` uses `asset.preview_url`; the copy handler and the displayed URL use `asset.url`.
- Next to the title, render `{asset.published && <Badge variant="secondary">{t("detail.published")}</Badge>}`, importing `Badge` from `@/components/ui/badge`.

Locales: in `frontend/src/locales/en/media.json`, replace `deleteDialog` and add `detail.published`. Keep the other `detail` keys and change `stableUrlTitle`:

```json
  "deleteDialog": {
    "title": "Delete this asset?",
    "description": "It will be removed from the library. Sent emails and the archive keep their published copy; drafts that use it will need a new file.",
    "unpublishLabel": "Also remove the published copy",
    "unpublishWarning": "The file disappears from emails already sent and from the archive. Use this for takedown requests.",
    "confirm": "Delete"
  },
```

```json
    "stableUrlTitle": "Library URL",
    "published": "Published"
```

In `frontend/src/locales/ru/media.json`:

```json
  "deleteDialog": {
    "title": "Удалить этот файл?",
    "description": "Файл будет удалён из библиотеки. Отправленные письма и архив сохранят опубликованную копию; черновикам с этим файлом понадобится новый.",
    "unpublishLabel": "Удалить и опубликованную копию",
    "unpublishWarning": "Файл пропадёт из уже отправленных писем и из архива. Используйте для запросов на удаление.",
    "confirm": "Удалить"
  },
```

```json
    "stableUrlTitle": "URL в библиотеке",
    "published": "Опубликован"
```

Regenerate the typed resources: `export PATH="$HOME/.nvm/versions/node/v24.19.0/bin:$PATH"; cd frontend && pnpm i18n:types`. `cd` persists between calls, so run `cd ..` afterwards.

- [ ] **Step 4: Editor, preview, branding, raw-HTML picker**

`frontend/src/components/visual-editor/extensions/ImageBlock.tsx`:
- In `addAttributes`, add `width: { default: null },`.
- In `renderHTML`, add `...(node.attrs.width ? { width: String(node.attrs.width) } : {}),` to the merged attributes.
- Change `applyImageBlockPick` to take and set a width:

```ts
export function applyImageBlockPick(
  editor: { commands: { command: (fn: (props: { tr: unknown; dispatch?: (tr: unknown) => void }) => boolean) => boolean } },
  pos: number,
  mediaRef: string,
  alt: string,
  width: number | null,
): boolean {
  return editor.commands.command(({ tr, dispatch }) => {
    if (!dispatch) return true
    const transaction = tr as {
      doc: { nodeAt: (pos: number) => { type: { name: string }; attrs: Record<string, unknown> } | null }
      setNodeMarkup: (pos: number, type: null, attrs: Record<string, unknown>) => unknown
    }
    const node = transaction.doc.nodeAt(pos)
    if (!node || node.type.name !== "image") return false
    transaction.setNodeMarkup(pos, null, { ...node.attrs, mediaRef, alt, width })
    return true
  })
}
```

`frontend/src/components/visual-editor/VisualEmailEditor.tsx`, `onImagePicked`:

```tsx
      applyImageBlockPick(
        editor,
        imagePick.pos,
        asset.url,
        asset.filename,
        imageDisplayWidth(asset.width),
      )
```

(import `imageDisplayWidth` from `@/lib/media-urls`).

`frontend/src/components/visual-editor/plugins/imageUpload.ts`:
- `replacePlaceholder(view, pendingRef, mediaRef, width: number | null)` sets `{ ...node.attrs, mediaRef, width }`.
- The call becomes `replacePlaceholder(view, pendingRef, res.data.url, imageDisplayWidth(res.data.width))`.
- Import `imageDisplayWidth` from `@/lib/media-urls`.

`frontend/src/components/visual-editor/ui/PreviewIframe.tsx`: import `usePreviewHtml` from `@/lib/media-urls`, and replace the existing `srcdoc` effect with:

```tsx
  // Library URLs need the session cookie, which the sandboxed (opaque-origin)
  // iframe never sends, so they are swapped for presigned preview URLs.
  const html = usePreviewHtml(slug, query.data?.bodyHtml)
  useEffect(() => {
    const iframe = iframeRef.current
    if (!iframe || html === undefined) return
    iframe.srcdoc = html
  }, [html])
```

`frontend/src/components/visual-editor/VisualBodyPreview.tsx` shows a draft's stored `body_html`, which holds library URLs, in a `sandbox=""` iframe. Give it a `slug` prop:
- The signature becomes `VisualBodyPreview({ slug, html, actions }: { slug: string; html: string; actions?: ReactNode })`.
- Add `const previewHtml = usePreviewHtml(slug, html)`.
- Use `srcDoc={previewHtml}`.
- In its two callers, `frontend/src/routes/t/$slug/campaigns/$id.tsx` and `frontend/src/routes/t/$slug/templates/$id.tsx`, pass `slug={slug}`. Both routes already have `slug` in scope; check with `grep -n "VisualBodyPreview" -A3`.
- Pass `slug="acme"` in `VisualBodyPreview.stories.tsx`.

`frontend/src/routes/t/$slug/branding/index.tsx`: `setForm((f) => ({ ...f, logo_url: asset.url }))`.

`frontend/src/routes/t/$slug/campaigns/$id.tsx`: `const insert = asset.url`.

- [ ] **Step 5: Update the existing tests and stories**

Find every remaining reference: `grep -rn "public_url" frontend/src`. In each fixture:
- Replace `public_url: "<X>"` with `url: "<X>", preview_url: "<X>?signature=test", published: false`. Upload results get only `url`.
- Assertions on thumbnails (`img` `src`) expect `preview_url`.
- Assertions on copy and links expect `url`.

In `frontend/src/routes/t/$slug/media/index.test.tsx`:
- Change the delete expectation to `expect(api.media.remove).toHaveBeenCalledWith("acme", "a1", { unpublish: false })`.
- Append:

```tsx
  it("offers to remove the published copy only for published assets", async () => {
    vi.mocked(api.media.list).mockResolvedValue(
      ok({ items: [asset({ id: "a1", published: true }), asset({ id: "a2" })] }),
    )
    vi.mocked(api.media.remove).mockResolvedValue(ok(null))
    renderWithClient(<MediaLibrary />)
    expect(await screen.findByTestId("media-grid")).toBeTruthy()

    fireEvent.click(screen.getByTestId("media-delete-a2"))
    let dialog = await screen.findByRole("alertdialog")
    expect(within(dialog).queryByRole("checkbox")).toBeNull()
    fireEvent.click(within(dialog).getByRole("button", { name: /cancel/i }))

    fireEvent.click(screen.getByTestId("media-delete-a1"))
    dialog = await screen.findByRole("alertdialog")
    fireEvent.click(within(dialog).getByRole("checkbox"))
    fireEvent.click(within(dialog).getByRole("button", { name: /delete/i }))
    await waitFor(() =>
      expect(api.media.remove).toHaveBeenCalledWith("acme", "a1", { unpublish: true }),
    )
  })
```

In `frontend/src/components/visual-editor/plugins/imageUpload.test.ts`:
- Rename `public_url` to `url` in the three mocked responses.
- In the first upload test, add `width: 1200` to the mocked `data`. After the existing `findImageNode` assertion, add:

```ts
    let width: unknown
    editor.state.doc.descendants((node) => {
      if (node.type.name === "image") width = node.attrs.width
    })
    expect(width).toBe(600)
```

- [ ] **Step 6: Run the frontend checks**

Run:
```bash
.claude/scripts/test-frontend.sh
export PATH="$HOME/.nvm/versions/node/v24.19.0/bin:$PATH"; cd frontend && pnpm typecheck && pnpm lint && pnpm i18n:lint; cd ..
```
Expected: all PASS. A typecheck error at a `public_url` or `applyImageBlockPick` call site means a site was missed; fix it as in Steps 3 and 4.

- [ ] **Step 7: Commit**

```bash
git add frontend/src
git commit -m "feat(frontend): use library URLs, presigned previews, image widths, and takedown"
```

---

### Task 12: Deployment config, docs, final verification

**Files:**
- Modify: `deploy/k8s/local/frontend.yaml`
- Modify: `.env.example`
- Modify: `docs/superpowers/specs/2026-11-14-media-library-design.md` (status line)

**Interfaces:**
- Consumes: everything above.
- Produces: the BFF receives `NVELOPE_BASE_URL` in local k8s; operators see the bucket requirement.

- [ ] **Step 1: Give the BFF its base URL**

In `deploy/k8s/local/frontend.yaml`, under the `frontend` container (after `ports:`):

```yaml
          env:
            # The BFF validates image mediaRefs against the tenant's library
            # URL prefix, built from the same base URL the Go API uses.
            - name: NVELOPE_BASE_URL
              valueFrom:
                configMapKeyRef:
                  name: nvelope-config
                  key: NVELOPE_BASE_URL
```

- [ ] **Step 2: Document the bucket requirement**

In `.env.example`, directly above `NVELOPE_OBJECT_STORAGE_PUBLIC_BASE_URL=`, add:

```bash
# The media bucket must NOT allow public read at bucket level: library
# objects (media/…) are private and members read them through presigned
# redirects. Only published copies (published/…), made when content goes
# live, are written with a public-read ACL. This base URL is where those
# public copies are reachable. The BFF reads NVELOPE_BASE_URL (above) to
# validate library URLs, so give the frontend the same value.
```

- [ ] **Step 3: Full verification**

Run:
```bash
export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock"; export PATH="$HOME/go/bin:$PATH"; make test && make lint-arch
.claude/scripts/test-frontend.sh
```
Expected: all PASS. `make lint-arch` confirms that no transport-layer package imports pgx or adapters, and that the media context does not import the campaign or tenant contexts (the bridges live in `internal/service`). For the known flaky tests, follow Global Constraints: re-run them, and report them as flaky if they still fail.

- [ ] **Step 4: Mark the spec implemented and commit**

In `docs/superpowers/specs/2026-11-14-media-library-design.md`, change `Status: Approved` to `Status: Implemented`.

```bash
git add deploy/k8s/local/frontend.yaml .env.example docs/superpowers/specs/2026-11-14-media-library-design.md
git commit -m "docs: document the private media bucket and mark the media library design implemented"
```

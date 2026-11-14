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

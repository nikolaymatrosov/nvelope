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
	if bytes.HasPrefix(data, []byte("%PDF-")) {
		return Processed{Data: data, ContentType: "application/pdf"}, nil
	}
	// image.DecodeConfig names the format from the registered decoders' magic
	// bytes; codecs is the allowlist. Unrecognized data yields "image/".
	_, format, _ := image.DecodeConfig(bytes.NewReader(data))
	ct := "image/" + format
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

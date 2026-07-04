package attach

import (
	"bytes"
	"image"
	"image/gif"

	// Register decoders for sniffing/decoding.
	_ "image/jpeg"
	_ "image/png"

	"github.com/gen2brain/webp"
	"github.com/rwcarlsen/goexif/exif"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// decodeConfig returns image dimensions without allocating pixels (bomb guard).
func decodeConfig(data []byte) (w, h int, format string, err error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, "", err
	}
	return cfg.Width, cfg.Height, format, nil
}

func decodeImage(data []byte) (image.Image, string, error) {
	return image.Decode(bytes.NewReader(data))
}

// encodeWebP encodes an image to lossy WebP at the given quality.
func encodeWebP(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := webp.Encode(&buf, img, webp.Options{Quality: quality, Method: 4}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// downscale shrinks img so its long edge is at most maxDim, using a sharp
// CatmullRom kernel. It never upscales. The second return reports whether a
// resize happened.
func downscale(img image.Image, maxDim int) (image.Image, bool) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	long := w
	if h > w {
		long = h
	}
	if maxDim <= 0 || long <= maxDim {
		return img, false
	}
	scale := float64(maxDim) / float64(long)
	nw := int(float64(w)*scale + 0.5)
	nh := int(float64(h)*scale + 0.5)
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, b, xdraw.Over, nil)
	return dst, true
}

// orientJPEG reads the EXIF orientation from the original JPEG bytes and bakes
// the rotation/flip into the decoded pixels. Re-encoding then drops all EXIF
// (including GPS) for free. Any parse failure is treated as orientation 1.
func orientJPEG(img image.Image, data []byte) image.Image {
	x, err := exif.Decode(bytes.NewReader(data))
	if err != nil {
		return img
	}
	tag, err := x.Get(exif.Orientation)
	if err != nil {
		return img
	}
	o, err := tag.Int(0)
	if err != nil {
		return img
	}
	return applyOrientation(img, o)
}

// applyOrientation returns img transformed for the given EXIF orientation (1-8).
func applyOrientation(img image.Image, o int) image.Image {
	switch o {
	case 2:
		return flipH(img)
	case 3:
		return rotate180(img)
	case 4:
		return flipV(img)
	case 5:
		return transpose(img)
	case 6:
		return rotate90(img)
	case 7:
		return transverse(img)
	case 8:
		return rotate270(img)
	default:
		return img
	}
}

func flipH(src image.Image) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(w-1-x, y, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

func flipV(src image.Image) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(x, h-1-y, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

func rotate180(src image.Image) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(w-1-x, h-1-y, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

func rotate90(src image.Image) *image.RGBA { // clockwise
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(h-1-y, x, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

func rotate270(src image.Image) *image.RGBA { // counter-clockwise
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(y, w-1-x, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

func transpose(src image.Image) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(y, x, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

func transverse(src image.Image) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(h-1-y, w-1-x, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

// isAnimatedGIF reports whether the GIF has more than one frame.
func isAnimatedGIF(data []byte) bool {
	g, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		return false
	}
	return len(g.Image) > 1
}

// isAnimatedWebP reports whether the WebP contains an animation chunk.
func isAnimatedWebP(data []byte) bool {
	return bytes.Contains(data, []byte("ANMF")) || bytes.Contains(data, []byte("ANIM"))
}

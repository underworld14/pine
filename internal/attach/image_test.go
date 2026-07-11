package attach

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/gif"
	"testing"
)

// markedImage builds a 3x2 RGBA image with a red pixel at the top-left (0,0)
// and a green pixel at the top-right (2,0), giving each orientation-transform
// test two independent landmarks to track.
func markedImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	img.Set(2, 0, color.RGBA{0, 255, 0, 255})
	return img
}

func isRedAt(img image.Image, x, y int) bool {
	r, g, b, _ := img.At(x, y).RGBA()
	return r>>8 > 200 && g>>8 < 50 && b>>8 < 50
}

func isGreenAt(img image.Image, x, y int) bool {
	r, g, b, _ := img.At(x, y).RGBA()
	return g>>8 > 200 && r>>8 < 50 && b>>8 < 50
}

func TestFlipH(t *testing.T) {
	out := flipH(markedImage())
	if out.Bounds().Dx() != 3 || out.Bounds().Dy() != 2 {
		t.Fatalf("dims = %dx%d, want 3x2", out.Bounds().Dx(), out.Bounds().Dy())
	}
	if !isRedAt(out, 2, 0) {
		t.Errorf("red pixel did not move to (2,0)")
	}
	if !isGreenAt(out, 0, 0) {
		t.Errorf("green pixel did not move to (0,0)")
	}
}

func TestFlipV(t *testing.T) {
	out := flipV(markedImage())
	if out.Bounds().Dx() != 3 || out.Bounds().Dy() != 2 {
		t.Fatalf("dims = %dx%d, want 3x2", out.Bounds().Dx(), out.Bounds().Dy())
	}
	if !isRedAt(out, 0, 1) {
		t.Errorf("red pixel did not move to (0,1)")
	}
	if !isGreenAt(out, 2, 1) {
		t.Errorf("green pixel did not move to (2,1)")
	}
}

func TestRotate180(t *testing.T) {
	out := rotate180(markedImage())
	if out.Bounds().Dx() != 3 || out.Bounds().Dy() != 2 {
		t.Fatalf("dims = %dx%d, want 3x2", out.Bounds().Dx(), out.Bounds().Dy())
	}
	if !isRedAt(out, 2, 1) {
		t.Errorf("red pixel did not move to (2,1)")
	}
	if !isGreenAt(out, 0, 1) {
		t.Errorf("green pixel did not move to (0,1)")
	}
}

func TestRotate270(t *testing.T) {
	out := rotate270(markedImage())
	if out.Bounds().Dx() != 2 || out.Bounds().Dy() != 3 {
		t.Fatalf("dims = %dx%d, want 2x3", out.Bounds().Dx(), out.Bounds().Dy())
	}
	if !isRedAt(out, 0, 2) {
		t.Errorf("red pixel did not move to (0,2)")
	}
	if !isGreenAt(out, 0, 0) {
		t.Errorf("green pixel did not move to (0,0)")
	}
}

func TestTranspose(t *testing.T) {
	out := transpose(markedImage())
	if out.Bounds().Dx() != 2 || out.Bounds().Dy() != 3 {
		t.Fatalf("dims = %dx%d, want 2x3", out.Bounds().Dx(), out.Bounds().Dy())
	}
	if !isRedAt(out, 0, 0) {
		t.Errorf("red pixel did not stay at (0,0)")
	}
	if !isGreenAt(out, 0, 2) {
		t.Errorf("green pixel did not move to (0,2)")
	}
}

func TestTransverse(t *testing.T) {
	out := transverse(markedImage())
	if out.Bounds().Dx() != 2 || out.Bounds().Dy() != 3 {
		t.Fatalf("dims = %dx%d, want 2x3", out.Bounds().Dx(), out.Bounds().Dy())
	}
	if !isRedAt(out, 1, 2) {
		t.Errorf("red pixel did not move to (1,2)")
	}
	if !isGreenAt(out, 1, 0) {
		t.Errorf("green pixel did not move to (1,0)")
	}
}

func TestApplyOrientationDispatch(t *testing.T) {
	// Orientation 1, and any unrecognized value, must be a no-op (default branch).
	for _, o := range []int{1, 0, 9} {
		out := applyOrientation(markedImage(), o)
		if !isRedAt(out, 0, 0) || !isGreenAt(out, 2, 0) {
			t.Errorf("orientation %d: expected no-op, landmarks moved", o)
		}
	}

	cases := []struct {
		o            int
		wantW, wantH int
	}{
		{2, 3, 2},
		{3, 3, 2},
		{4, 3, 2},
		{5, 2, 3},
		{7, 2, 3},
		{8, 2, 3},
	}
	for _, c := range cases {
		out := applyOrientation(markedImage(), c.o)
		b := out.Bounds()
		if b.Dx() != c.wantW || b.Dy() != c.wantH {
			t.Errorf("orientation %d: dims = %dx%d, want %dx%d", c.o, b.Dx(), b.Dy(), c.wantW, c.wantH)
		}
		// Orientations 2-4 keep the original dimensions, so also confirm the
		// pixels actually moved (distinguishing them from the orientation-1
		// no-op above). 5/7/8 swap width and height, which the dims check
		// above already distinguishes from a no-op; per-transform pixel
		// movement for those is covered by TestTranspose/TestTransverse/
		// TestRotate270 directly.
		if c.o == 2 || c.o == 3 || c.o == 4 {
			if isRedAt(out, 0, 0) {
				t.Errorf("orientation %d: expected red landmark to move off (0,0)", c.o)
			}
		}
	}
}

// buildTIFFOrientation constructs the smallest possible little-endian TIFF
// byte stream containing a single Orientation (0x0112) SHORT tag. goexif's
// Decode accepts raw TIFF (it detects the "II*\x00"/"MM\x00*" header) as well
// as full JPEGs, so this sidesteps needing to hand-craft a JPEG APP1/EXIF
// segment while still exercising orientJPEG's real parsing path.
func buildTIFFOrientation(o uint16) []byte {
	buf := make([]byte, 26)
	copy(buf[0:4], []byte("II*\x00"))
	binary.LittleEndian.PutUint32(buf[4:8], 8)        // offset to IFD0
	binary.LittleEndian.PutUint16(buf[8:10], 1)       // 1 entry
	binary.LittleEndian.PutUint16(buf[10:12], 0x0112) // Orientation tag
	binary.LittleEndian.PutUint16(buf[12:14], 3)      // type SHORT
	binary.LittleEndian.PutUint32(buf[14:18], 1)      // count
	binary.LittleEndian.PutUint16(buf[18:20], o)      // value (padded to 4 bytes)
	binary.LittleEndian.PutUint32(buf[22:26], 0)      // next IFD offset
	return buf
}

// buildTIFFNoOrientation is a valid but empty IFD0 (no Orientation tag).
func buildTIFFNoOrientation() []byte {
	buf := make([]byte, 14)
	copy(buf[0:4], []byte("II*\x00"))
	binary.LittleEndian.PutUint32(buf[4:8], 8)   // offset to IFD0
	binary.LittleEndian.PutUint16(buf[8:10], 0)  // 0 entries
	binary.LittleEndian.PutUint32(buf[10:14], 0) // next IFD offset
	return buf
}

func TestOrientJPEGAppliesOrientation(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 2))
	out := orientJPEG(img, buildTIFFOrientation(6)) // rotate 90 CW
	b := out.Bounds()
	if b.Dx() != 2 || b.Dy() != 4 {
		t.Fatalf("dims = %dx%d, want 2x4", b.Dx(), b.Dy())
	}
}

func TestOrientJPEGNoOrientationTag(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 2))
	out := orientJPEG(img, buildTIFFNoOrientation())
	if out != image.Image(img) {
		t.Errorf("expected the same image back when the orientation tag is absent")
	}
}

func TestOrientJPEGInvalidData(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 2))
	out := orientJPEG(img, []byte("not exif or tiff data at all"))
	if out != image.Image(img) {
		t.Errorf("expected the same image back when EXIF parsing fails")
	}
}

func TestIsAnimatedWebP(t *testing.T) {
	if !isAnimatedWebP([]byte("RIFFxxxxWEBPVP8XxxxxANIMxxx")) {
		t.Errorf("expected ANIM chunk to be detected as animated")
	}
	if !isAnimatedWebP([]byte("RIFFxxxxWEBPVP8XxxxxANMFxxx")) {
		t.Errorf("expected ANMF chunk to be detected as animated")
	}
	if isAnimatedWebP([]byte("RIFFxxxxWEBPVP8 xxxxxxxxxxxx")) {
		t.Errorf("static webp should not be detected as animated")
	}
}

func TestIsAnimatedGIFInvalidData(t *testing.T) {
	if isAnimatedGIF([]byte("not a gif at all")) {
		t.Errorf("invalid data should not be treated as animated")
	}
}

func TestIsAnimatedGIFSingleFrame(t *testing.T) {
	pal := color.Palette{color.Black, color.White}
	single := image.NewPaletted(image.Rect(0, 0, 4, 4), pal)
	var buf bytes.Buffer
	if err := gif.Encode(&buf, single, nil); err != nil {
		t.Fatal(err)
	}
	if isAnimatedGIF(buf.Bytes()) {
		t.Errorf("single-frame gif should not be animated")
	}
}

func TestDecodeConfigError(t *testing.T) {
	if _, _, _, err := decodeConfig([]byte("not an image")); err == nil {
		t.Errorf("expected an error decoding invalid image data")
	}
}

func TestEncodeWebPError(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 0, 0))
	if _, err := encodeWebP(img, 80); err == nil {
		t.Errorf("expected an error encoding a zero-size image")
	}
}

func TestDownscaleNoopWhenMaxDimZero(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 50))
	out, scaled := downscale(img, 0)
	if scaled {
		t.Errorf("expected no scaling when maxDim <= 0")
	}
	if out != image.Image(img) {
		t.Errorf("expected the same image back")
	}
}

func TestDownscaleNoopWithinBounds(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 50))
	out, scaled := downscale(img, 200)
	if scaled {
		t.Errorf("expected no scaling when already within bounds")
	}
	if out != image.Image(img) {
		t.Errorf("expected the same image back")
	}
}

func TestDownscalePortrait(t *testing.T) {
	// Height exceeds width, exercising the h > w branch that picks the long edge.
	img := image.NewRGBA(image.Rect(0, 0, 100, 300))
	out, scaled := downscale(img, 150)
	if !scaled {
		t.Fatalf("expected scaling for an oversized portrait image")
	}
	b := out.Bounds()
	if b.Dy() != 150 {
		t.Errorf("long edge (height) = %d, want 150", b.Dy())
	}
	if b.Dx() != 50 {
		t.Errorf("short edge (width) = %d, want 50", b.Dx())
	}
}

package attach

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"testing"

	"github.com/underworld14/pine/internal/config"
)

// gradientImage builds an in-memory RGBA gradient, useful as encoder input
// (as opposed to gradientPNG in attach_test.go, which returns encoded bytes).
func gradientImage(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 3), uint8(y * 3), uint8((x + y) * 2), 255})
		}
	}
	return img
}

// jpegBytes encodes a non-trivial JPEG (no EXIF) of the given size.
func jpegBytes(w, h int) []byte {
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, gradientImage(w, h), &jpeg.Options{Quality: 90})
	return buf.Bytes()
}

// fakePNGHeader builds the minimal valid PNG prefix (signature + IHDR chunk,
// including a correct CRC) that satisfies image.DecodeConfig without any
// pixel data behind it. The Go PNG decoder's configOnly path returns as soon
// as it has parsed a non-paletted IHDR (it still reads and verifies that
// chunk's CRC before returning), so this is enough to drive decodeConfig
// successfully while still failing a full decode (no IDAT/IEND follow).
func fakePNGHeader(w, h uint32) []byte {
	buf := make([]byte, 8+8+13+4)
	copy(buf[0:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	binary.BigEndian.PutUint32(buf[8:12], 13)
	copy(buf[12:16], []byte("IHDR"))
	binary.BigEndian.PutUint32(buf[16:20], w)
	binary.BigEndian.PutUint32(buf[20:24], h)
	buf[24] = 8 // bit depth
	buf[25] = 2 // color type: truecolor (non-paletted, so config decode stops here)
	buf[26] = 0 // compression method
	buf[27] = 0 // filter method
	buf[28] = 0 // interlace method
	crc := crc32.ChecksumIEEE(buf[12:29])
	binary.BigEndian.PutUint32(buf[29:33], crc)
	return buf
}

func TestFromConfig(t *testing.T) {
	cfg := config.Attachments{Optimize: true, MaxDimension: 1600, Quality: 75, MaxVideoMB: 25}
	want := Config{Optimize: true, MaxDimension: 1600, Quality: 75, MaxVideoMB: 25}
	if got := FromConfig(cfg); got != want {
		t.Errorf("FromConfig(%+v) = %+v, want %+v", cfg, got, want)
	}

	if got := FromConfig(config.Attachments{}); got != (Config{}) {
		t.Errorf("FromConfig(zero) = %+v, want zero Config", got)
	}
}

func TestHumanBytesUnderUnit(t *testing.T) {
	if got := humanBytes(500); got != "500 B" {
		t.Errorf("humanBytes(500) = %q, want %q", got, "500 B")
	}
}

func TestHumanBytesGigabytes(t *testing.T) {
	got := humanBytes(2 * 1024 * 1024 * 1024)
	if !strings.Contains(got, "GB") {
		t.Errorf("humanBytes(2GB) = %q, want it to contain GB", got)
	}
}

func TestPassthroughVideoKind(t *testing.T) {
	p := passthrough("clip", ".mov", []byte("fake video bytes"), "video/quicktime")
	if p.Kind != "video" {
		t.Errorf("passthrough(video/quicktime) kind = %q, want video", p.Kind)
	}
}

func TestSlugEdgeCases(t *testing.T) {
	if got := slug(""); got != "paste" {
		t.Errorf("slug(\"\") = %q, want paste", got)
	}
	if got := slug("!!!???"); got != "paste" {
		t.Errorf("slug(all-invalid) = %q, want paste", got)
	}
	long := strings.Repeat("a", 60)
	if got := slug(long); len(got) > 48 {
		t.Errorf("slug(long) not truncated: len=%d", len(got))
	}
}

func TestProcessErrTooLarge(t *testing.T) {
	// 10000x6000 = 60,000,000 pixels, over the 50,000,000 decode-bomb guard.
	data := fakePNGHeader(10000, 6000)
	if _, err := Process("huge.png", data, defaultCfg()); !errors.Is(err, ErrTooLarge) {
		t.Errorf("Process(huge) err = %v, want ErrTooLarge", err)
	}
}

func TestProcessErrDecode(t *testing.T) {
	// Width exceeds cfg.MaxDimension so the tiny/in-bounds passthrough is
	// skipped, but there is no pixel data behind the header, so the full
	// decode must fail.
	data := fakePNGHeader(3000, 100)
	_, err := Process("broken.png", data, defaultCfg())
	if !errors.Is(err, ErrDecode) {
		t.Errorf("Process(broken) err = %v, want ErrDecode", err)
	}
}

func TestProcessJPEGGoesThroughOrientPath(t *testing.T) {
	// Large enough to force the full decode->orient->downscale->encode path
	// (rather than the tiny/in-bounds passthrough), exercising Process's
	// `if format == "jpeg" { img = orientJPEG(img, data) }` branch.
	data := jpegBytes(3000, 1500)
	p, err := Process("photo.jpg", data, defaultCfg())
	if err != nil {
		t.Fatal(err)
	}
	if !p.Optimized {
		t.Errorf("expected optimized output for oversized jpeg")
	}
	if p.Width != 2000 {
		t.Errorf("width = %d, want 2000", p.Width)
	}
}

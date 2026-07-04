// Package attach ingests uploaded images and videos: it sniffs the real content
// type, optionally optimizes images (EXIF-orient, downscale, re-encode to lossy
// WebP, keep-smaller), and returns the bytes to persist plus metadata. It never
// touches the filesystem, so it is pure and easily tested; the caller writes the
// returned bytes through the store.
package attach

import (
	"errors"
	"fmt"

	"github.com/underworld14/pine/internal/config"
)

// Config controls optimization, derived from config.json's attachments block.
type Config struct {
	Optimize     bool
	MaxDimension int
	Quality      int
	MaxVideoMB   int
}

// FromConfig adapts the persisted attachments config.
func FromConfig(a config.Attachments) Config {
	return Config{
		Optimize:     a.Optimize,
		MaxDimension: a.MaxDimension,
		Quality:      a.Quality,
		MaxVideoMB:   a.MaxVideoMB,
	}
}

// Processed is the outcome of ingesting one file.
type Processed struct {
	FileName      string
	Data          []byte
	Mime          string
	Kind          string // image | video
	OriginalBytes int64
	FinalBytes    int64
	Width         int
	Height        int
	Optimized     bool
	Warning       string
}

// Errors surfaced to HTTP handlers.
var (
	ErrUnsupported = errors.New("unsupported file type")
	ErrTooLarge    = errors.New("image is too large (over 50 megapixels)")
	ErrDecode      = errors.New("could not decode image")
)

const (
	maxPixels     = 50_000_000 // decode-bomb guard
	tinyThreshold = 32 * 1024  // below this an in-bounds image is left as-is
)

// Process ingests one file. clientName seeds the filename; empty names (pasted
// blobs) become "paste". The returned FileName is content-addressed so the same
// bytes always map to the same name (free dedup).
func Process(clientName string, data []byte, cfg Config) (Processed, error) {
	mime, kind, ext, ok := Sniff(data)
	if !ok {
		return Processed{}, ErrUnsupported
	}
	orig := int64(len(data))
	base := slug(clientName)

	if kind == "video" {
		var warn string
		if cfg.MaxVideoMB > 0 && orig > int64(cfg.MaxVideoMB)*1024*1024 {
			warn = fmt.Sprintf("video is %s (recommended max %d MB); large files bloat the git repo",
				humanBytes(orig), cfg.MaxVideoMB)
		}
		return Processed{
			FileName:      hashName(base, ext, data),
			Data:          data,
			Mime:          mime,
			Kind:          "video",
			OriginalBytes: orig,
			FinalBytes:    orig,
			Optimized:     false,
			Warning:       warn,
		}, nil
	}

	// Images. Passthrough cases first.
	if !cfg.Optimize ||
		(ext == ".gif" && isAnimatedGIF(data)) ||
		(ext == ".webp" && isAnimatedWebP(data)) {
		return passthrough(base, ext, data, mime), nil
	}

	w0, h0, _, cfgErr := decodeConfig(data)
	if cfgErr == nil && int64(w0)*int64(h0) > maxPixels {
		return Processed{}, ErrTooLarge
	}
	if len(data) < tinyThreshold && cfgErr == nil && w0 <= cfg.MaxDimension && h0 <= cfg.MaxDimension {
		return passthrough(base, ext, data, mime), nil
	}

	img, format, err := decodeImage(data)
	if err != nil {
		return Processed{}, fmt.Errorf("%w: %v", ErrDecode, err)
	}
	if format == "jpeg" {
		img = orientJPEG(img, data)
	}
	scaled, didScale := downscale(img, cfg.MaxDimension)

	webpBytes, err := encodeWebP(scaled, cfg.Quality)
	if err != nil {
		// Optimization must never fail an upload: keep the original.
		return passthrough(base, ext, data, mime), nil
	}
	// Keep-smaller rule only applies when no downscale happened (a downscaled
	// original is a different, larger image, so byte comparison is meaningless).
	if !didScale && len(webpBytes) >= len(data) {
		return passthrough(base, ext, data, mime), nil
	}

	b := scaled.Bounds()
	return Processed{
		FileName:      hashName(base, ".webp", data),
		Data:          webpBytes,
		Mime:          "image/webp",
		Kind:          "image",
		OriginalBytes: orig,
		FinalBytes:    int64(len(webpBytes)),
		Width:         b.Dx(),
		Height:        b.Dy(),
		Optimized:     true,
	}, nil
}

func passthrough(base, ext string, data []byte, mime string) Processed {
	w, h := 0, 0
	if wc, hc, _, err := decodeConfig(data); err == nil {
		w, h = wc, hc
	}
	kind := "image"
	if mime == "video/mp4" || mime == "video/quicktime" {
		kind = "video"
	}
	return Processed{
		FileName:      hashName(base, ext, data),
		Data:          data,
		Mime:          mime,
		Kind:          kind,
		OriginalBytes: int64(len(data)),
		FinalBytes:    int64(len(data)),
		Width:         w,
		Height:        h,
		Optimized:     false,
	}
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}

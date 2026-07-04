package attach

import (
	"net/http"
	"strings"
)

// Sniff detects an uploaded file's real type from its magic bytes (never the
// client-provided extension or MIME). It returns the canonical MIME, coarse kind
// ("image"/"video"), a normalized extension, and whether the type is supported.
func Sniff(data []byte) (mime, kind, ext string, ok bool) {
	if m, e := sniffVideo(data); m != "" {
		return m, "video", e, true
	}
	switch http.DetectContentType(data) {
	case "image/png":
		return "image/png", "image", ".png", true
	case "image/jpeg":
		return "image/jpeg", "image", ".jpg", true
	case "image/gif":
		return "image/gif", "image", ".gif", true
	case "image/webp":
		return "image/webp", "image", ".webp", true
	}
	return "", "", "", false
}

// sniffVideo recognizes MP4/MOV by the ISO base media "ftyp" box brand. SVG and
// other formats are intentionally not recognized here (and thus rejected).
func sniffVideo(data []byte) (mime, ext string) {
	if len(data) < 12 || string(data[4:8]) != "ftyp" {
		return "", ""
	}
	brand := string(data[8:12])
	if strings.HasPrefix(brand, "qt") {
		return "video/quicktime", ".mov"
	}
	return "video/mp4", ".mp4"
}

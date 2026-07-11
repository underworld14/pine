package attach

import "testing"

func TestSniffVideoQuickTime(t *testing.T) {
	data := make([]byte, 16)
	copy(data[4:8], []byte("ftyp"))
	copy(data[8:12], []byte("qt  "))
	mime, ext := sniffVideo(data)
	if mime != "video/quicktime" || ext != ".mov" {
		t.Errorf("sniffVideo(qt brand) = %q %q, want video/quicktime .mov", mime, ext)
	}
}

func TestSniffVideoTooShort(t *testing.T) {
	if mime, ext := sniffVideo([]byte("short")); mime != "" || ext != "" {
		t.Errorf("sniffVideo(short) = %q %q, want empty", mime, ext)
	}
}

func TestSniffDetectsJPEG(t *testing.T) {
	data := jpegBytes(10, 10)
	mime, kind, ext, ok := Sniff(data)
	if !ok || mime != "image/jpeg" || kind != "image" || ext != ".jpg" {
		t.Errorf("Sniff(jpeg) = %q %q %q %v, want image/jpeg image .jpg true", mime, kind, ext, ok)
	}
}

func TestSniffDetectsWebP(t *testing.T) {
	data, err := encodeWebP(gradientImage(10, 10), 80)
	if err != nil {
		t.Fatal(err)
	}
	mime, kind, ext, ok := Sniff(data)
	if !ok || mime != "image/webp" || kind != "image" || ext != ".webp" {
		t.Errorf("Sniff(webp) = %q %q %q %v, want image/webp image .webp true", mime, kind, ext, ok)
	}
}

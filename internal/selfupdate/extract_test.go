package selfupdate

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestExtractBinaryZip(t *testing.T) {
	t.Parallel()
	payload := []byte("windows-bin")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("pine.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := ExtractBinary(buf.Bytes(), "pine_1.0.0_windows_amd64.zip", "windows")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("got %q", got)
	}
}

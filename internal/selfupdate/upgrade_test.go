package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCheckAndUpgradeWithMockGitHub(t *testing.T) {
	payload := []byte("new-pine-binary-v0.9.0")
	archiveName := fmt.Sprintf("pine_0.9.0_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		t.Skip("archive helper builds tar.gz; windows zip covered in extract tests")
	}
	archive := mustTarGz(t, "pine", payload)
	sum := sha256.Sum256(archive)
	checksums := hex.EncodeToString(sum[:]) + "  " + archiveName + "\n"

	var serverURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/underworld14/pine/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Release{
			TagName: "v0.9.0",
			Assets: []Asset{
				{Name: archiveName, BrowserDownloadURL: serverURL + "/download/" + archiveName},
				{Name: "checksums.txt", BrowserDownloadURL: serverURL + "/download/checksums.txt"},
			},
		})
	})
	mux.HandleFunc("/download/"+archiveName, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	})
	mux.HandleFunc("/download/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(checksums))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	serverURL = ts.URL

	client := &Client{
		HTTP:    ts.Client(),
		APIBase: ts.URL,
		Owner:   DefaultOwner,
		Repo:    DefaultRepo,
		Version: "0.1.0-dev",
	}

	ctx := context.Background()
	checkRes, err := Check(ctx, Options{
		CurrentVersion: "0.8.0",
		GOOS:           runtime.GOOS,
		GOARCH:         runtime.GOARCH,
		Client:         client,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !checkRes.NeedsUpdate || checkRes.Latest != "0.9.0" {
		t.Fatalf("check: %+v", checkRes)
	}

	// Up to date
	okRes, err := Check(ctx, Options{
		CurrentVersion: "0.9.0",
		GOOS:           runtime.GOOS,
		GOARCH:         runtime.GOARCH,
		Client:         client,
	})
	if err != nil {
		t.Fatal(err)
	}
	if okRes.NeedsUpdate {
		t.Fatalf("expected up to date: %+v", okRes)
	}

	dir := t.TempDir()
	exe := filepath.Join(dir, "pine")
	if err := os.WriteFile(exe, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	upRes, err := Upgrade(ctx, Options{
		CurrentVersion: "0.8.0",
		Executable:     exe,
		GOOS:           runtime.GOOS,
		GOARCH:         runtime.GOARCH,
		Client:         client,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !upRes.Updated {
		t.Fatalf("expected Updated: %+v", upRes)
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("executable content = %q, want %q", got, payload)
	}

	// Force reinstall when versions match
	if err := os.WriteFile(exe, []byte("stale"), 0o755); err != nil {
		t.Fatal(err)
	}
	forceRes, err := Upgrade(ctx, Options{
		CurrentVersion: "0.9.0",
		Force:          true,
		Executable:     exe,
		GOOS:           runtime.GOOS,
		GOARCH:         runtime.GOARCH,
		Client:         client,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !forceRes.Updated {
		t.Fatal("force should reinstall")
	}
	got, _ = os.ReadFile(exe)
	if !bytes.Equal(got, payload) {
		t.Fatalf("after force: %q", got)
	}
}

func TestExtractBinaryTarGz(t *testing.T) {
	t.Parallel()
	payload := []byte("bin-contents")
	archive := mustTarGz(t, "pine", payload)
	got, err := ExtractBinary(archive, "pine_1.0.0_linux_amd64.tar.gz", "linux")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("got %q", got)
	}
}

func TestReplaceExecutable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "pine")
	if err := os.WriteFile(path, []byte("v1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceExecutable(path, []byte("v2")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "v2" {
		t.Fatalf("got %q", got)
	}
	if _, err := os.Stat(path + ".old"); !os.IsNotExist(err) {
		t.Fatalf("backup should be removed: %v", err)
	}
}

func mustTarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	hdr := &tar.Header{
		Name: name,
		Mode: 0o755,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

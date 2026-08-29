package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/underworld14/pine/internal/selfupdate"
)

func TestUpgradeHelp(t *testing.T) {
	dir := t.TempDir()
	out, err := run(t, dir, "upgrade", "--help")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "GitHub") || !strings.Contains(out, "--check") {
		t.Fatalf("help:\n%s", out)
	}
}

func TestUpdateStillTicketCommand(t *testing.T) {
	out, err := run(t, t.TempDir(), "update", "--help")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Update a ticket") {
		t.Fatalf("update should still be ticket mutation:\n%s", out)
	}
}

func TestUpgradeCheckCLI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("tar.gz mock")
	}
	ts := startMockRelease(t, []byte("bin"))
	defer ts.Close()

	prevClient := upgradeNewClient
	prevVer := version
	upgradeNewClient = func(v string) *selfupdate.Client {
		return &selfupdate.Client{
			HTTP: ts.Client(), APIBase: ts.URL,
			Owner: selfupdate.DefaultOwner, Repo: selfupdate.DefaultRepo, Version: v,
		}
	}
	version = "0.8.0"
	t.Cleanup(func() {
		upgradeNewClient = prevClient
		version = prevVer
	})

	out, err := run(t, t.TempDir(), "upgrade", "--check")
	if !errors.Is(err, errUpdateAvailable) {
		t.Fatalf("err = %v, want errUpdateAvailable; out:\n%s", err, out)
	}
	if !strings.Contains(out, "latest:  0.9.0") || !strings.Contains(out, "update available") {
		t.Fatalf("check output:\n%s", out)
	}

	version = "0.9.0"
	out, err = run(t, t.TempDir(), "upgrade", "--check")
	if err != nil {
		t.Fatalf("up to date should succeed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "up to date") {
		t.Fatalf("output:\n%s", out)
	}
}

func startMockRelease(t *testing.T, payload []byte) *httptest.Server {
	t.Helper()
	archiveName := fmt.Sprintf("pine_0.9.0_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	archive := mustCLITarGz(t, "pine", payload)
	sum := sha256.Sum256(archive)
	checksums := hex.EncodeToString(sum[:]) + "  " + archiveName + "\n"

	var serverURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/underworld14/pine/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(selfupdate.Release{
			TagName: "v0.9.0",
			Assets: []selfupdate.Asset{
				{Name: archiveName, BrowserDownloadURL: serverURL + "/d/" + archiveName},
				{Name: "checksums.txt", BrowserDownloadURL: serverURL + "/d/checksums.txt"},
			},
		})
	})
	mux.HandleFunc("/d/"+archiveName, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(archive) })
	mux.HandleFunc("/d/checksums.txt", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(checksums)) })
	ts := httptest.NewServer(mux)
	serverURL = ts.URL
	return ts
}

func mustCLITarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	hdr := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(content))}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	_ = tw.Close()
	_ = gw.Close()
	return buf.Bytes()
}

func TestUpgradeForceViaSelfupdate(t *testing.T) {
	// Full replace path covered in internal/selfupdate; keep a thin CLI-adjacent
	// assertion that the mock release server is coherent.
	if runtime.GOOS == "windows" {
		t.Skip("tar.gz mock")
	}
	payload := []byte("upgraded-binary")
	ts := startMockRelease(t, payload)
	defer ts.Close()

	dir := t.TempDir()
	exe := filepath.Join(dir, "pine")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	res, err := selfupdate.Upgrade(t.Context(), selfupdate.Options{
		CurrentVersion: "0.1.0-dev",
		Executable:     exe,
		GOOS:           runtime.GOOS,
		GOARCH:         runtime.GOARCH,
		Client: &selfupdate.Client{
			HTTP: ts.Client(), APIBase: ts.URL,
			Owner: selfupdate.DefaultOwner, Repo: selfupdate.DefaultRepo,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Updated {
		t.Fatalf("%+v", res)
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("got %q", got)
	}
}

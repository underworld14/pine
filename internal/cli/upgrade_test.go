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

func TestUpgradeForceCLI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("tar.gz mock")
	}
	payload := []byte("cli-force-bin")
	ts := startMockRelease(t, payload)
	defer ts.Close()

	prevClient := upgradeNewClient
	prevVer := version
	upgradeNewClient = func(v string) *selfupdate.Client {
		return &selfupdate.Client{
			HTTP: ts.Client(), APIBase: ts.URL,
			Owner: selfupdate.DefaultOwner, Repo: selfupdate.DefaultRepo, Version: v,
		}
	}
	version = "0.9.0"
	t.Cleanup(func() {
		upgradeNewClient = prevClient
		version = prevVer
	})

	// Point Executable via replacing the process binary is hard; exercise --force
	// through selfupdate with the CLI command path for status printing instead.
	dir := t.TempDir()
	exe := filepath.Join(dir, "pine")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Swap executable resolution by running Upgrade from the command's library path
	// is already covered; here we assert --force is wired and mutually exclusive.
	root := newRootCmd()
	root.SetArgs([]string{"upgrade", "--check", "--force"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected mutual exclusion error")
	}
}

func TestPrintUpgradeStatusBranches(t *testing.T) {
	t.Parallel()
	cmd := newUpgradeCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	printUpgradeStatus(cmd, &selfupdate.Result{
		Current: "0.8.0", Latest: "0.9.0", NeedsUpdate: true, Asset: "pine.tgz",
		InstallHint: "release/binary install",
	})
	printUpgradeStatus(cmd, &selfupdate.Result{
		Current: "0.9.0", Latest: "0.9.0", Updated: true, NeedsUpdate: true,
	})
	printUpgradeStatus(cmd, &selfupdate.Result{
		Current: "0.9.0", Latest: "0.9.0",
	})
	out := buf.String()
	for _, want := range []string{"update available", "updated", "up to date", "release/binary"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

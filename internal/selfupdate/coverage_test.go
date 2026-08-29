package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
)

func TestCurrentAssetName(t *testing.T) {
	t.Parallel()
	got, err := CurrentAssetName("v0.8.0")
	if err != nil {
		t.Fatal(err)
	}
	want, err := AssetName("0.8.0", runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if _, err := CurrentAssetName(""); err == nil {
		t.Fatal("expected empty version error")
	}
}

func TestNewClientDefaults(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "from-gh")
	c := NewClient("0.8.0")
	if c.Owner != DefaultOwner || c.Repo != DefaultRepo {
		t.Fatalf("owner/repo: %+v", c)
	}
	if c.Token != "from-gh" {
		t.Fatalf("token = %q", c.Token)
	}
	if c.APIBase != defaultAPI || c.Version != "0.8.0" {
		t.Fatalf("%+v", c)
	}
	t.Setenv("GITHUB_TOKEN", "from-github")
	c2 := NewClient("1.0.0")
	if c2.Token != "from-github" {
		t.Fatalf("GITHUB_TOKEN should win: %q", c2.Token)
	}
}

func TestClientHelpersDefaults(t *testing.T) {
	t.Parallel()
	c := &Client{}
	if c.http() != http.DefaultClient {
		t.Fatal("http default")
	}
	if c.apiBase() != defaultAPI {
		t.Fatal("apiBase default")
	}
	if c.owner() != DefaultOwner || c.repo() != DefaultRepo {
		t.Fatal("owner/repo default")
	}
	req, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
	c.Token = "tok"
	c.Version = "9.9.9"
	c.setHeaders(req)
	if req.Header.Get("Authorization") != "Bearer tok" {
		t.Fatal(req.Header.Get("Authorization"))
	}
	if req.Header.Get("User-Agent") != "pine/9.9.9" {
		t.Fatal(req.Header.Get("User-Agent"))
	}
}

func TestFindAssetURLMiss(t *testing.T) {
	t.Parallel()
	r := &Release{Assets: []Asset{{Name: "other", BrowserDownloadURL: "u"}}}
	if r.FindAssetURL("missing") != "" {
		t.Fatal("expected empty")
	}
}

func TestLatestReleaseAndDownloadErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/underworld14/pine/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	c := &Client{HTTP: ts.Client(), APIBase: ts.URL, Owner: DefaultOwner, Repo: DefaultRepo}
	if _, err := c.LatestRelease(context.Background()); err == nil {
		t.Fatal("expected API error")
	}

	mux2 := http.NewServeMux()
	mux2.HandleFunc("/repos/underworld14/pine/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Release{TagName: ""})
	})
	ts2 := httptest.NewServer(mux2)
	defer ts2.Close()
	c2 := &Client{HTTP: ts2.Client(), APIBase: ts2.URL, Owner: DefaultOwner, Repo: DefaultRepo}
	if _, err := c2.LatestRelease(context.Background()); err == nil {
		t.Fatal("expected empty tag error")
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusNotFound)
	}))
	defer bad.Close()
	if _, err := c.Download(context.Background(), bad.URL); err == nil {
		t.Fatal("expected download error")
	}
}

func TestUpgradeAlreadyUpToDate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("tar.gz mock")
	}
	archiveName := "pine_0.9.0_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"
	archive := mustTarGz(t, "pine", []byte("x"))
	var serverURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/underworld14/pine/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Release{
			TagName: "v0.9.0",
			Assets: []Asset{
				{Name: archiveName, BrowserDownloadURL: serverURL + "/a"},
				{Name: "checksums.txt", BrowserDownloadURL: serverURL + "/c"},
			},
		})
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	serverURL = ts.URL
	client := &Client{HTTP: ts.Client(), APIBase: ts.URL, Owner: DefaultOwner, Repo: DefaultRepo}
	res, err := Upgrade(context.Background(), Options{
		CurrentVersion: "0.9.0",
		GOOS:           runtime.GOOS,
		GOARCH:         runtime.GOARCH,
		Client:         client,
		Executable:     filepath.Join(t.TempDir(), "unused"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated || res.NeedsUpdate {
		t.Fatalf("%+v", res)
	}
	_ = archive
}

func TestReplaceExecutableErrorsAndMoveAside(t *testing.T) {
	t.Parallel()
	if err := ReplaceExecutable("", []byte("x")); err == nil {
		t.Fatal("empty path")
	}
	dir := t.TempDir()
	if err := ReplaceExecutable(dir, []byte("x")); err == nil {
		t.Fatal("directory not regular")
	}

	path := filepath.Join(dir, "pine")
	if err := os.WriteFile(path, []byte("v1"), 0o755); err != nil {
		t.Fatal(err)
	}
	tmp, err := os.CreateTemp(dir, ".pine-upgrade-*")
	if err != nil {
		t.Fatal(err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write([]byte("v2")); err != nil {
		t.Fatal(err)
	}
	_ = tmp.Close()
	if err := replaceViaMoveAside(path, tmpPath); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "v2" {
		t.Fatalf("got %q", got)
	}
}

func TestIsBusyRenameError(t *testing.T) {
	t.Parallel()
	if isBusyRenameError(nil) {
		t.Fatal("nil")
	}
	if !isBusyRenameError(syscall.ETXTBSY) {
		t.Fatal("ETXTBSY")
	}
	if !isBusyRenameError(syscall.EBUSY) {
		t.Fatal("EBUSY")
	}
	if isBusyRenameError(errors.New("other")) {
		t.Fatal("other")
	}
}

func TestExecutablePath(t *testing.T) {
	t.Parallel()
	if _, err := ExecutablePath(); err != nil {
		t.Fatal(err)
	}
}

func TestExtractBinaryErrors(t *testing.T) {
	t.Parallel()
	if _, err := ExtractBinary(nil, "pine.bin", "linux"); err == nil {
		t.Fatal("unsupported format")
	}
	// tar.gz without the wanted binary
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	_ = tw.WriteHeader(&tar.Header{Name: "README.md", Mode: 0o644, Size: 4})
	_, _ = tw.Write([]byte("docs"))
	_ = tw.Close()
	_ = gw.Close()
	if _, err := ExtractBinary(buf.Bytes(), "pine_1.0.0_linux_amd64.tar.gz", "linux"); err == nil {
		t.Fatal("expected missing binary")
	}
	if _, err := ExtractBinary([]byte("not-a-zip"), "pine.zip", "windows"); err == nil {
		t.Fatal("expected zip error")
	}
}

func TestInstallHint(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GOPATH", "")
	t.Setenv("GOBIN", "")
	goBin := filepath.Join(home, "go", "bin")
	if err := os.MkdirAll(goBin, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(goBin, "pine")
	if err := os.WriteFile(exe, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	hint := installHint(Options{Executable: exe})
	if hint == "" || hint == "release/binary install" {
		t.Fatalf("want go install hint, got %q", hint)
	}

	other := filepath.Join(t.TempDir(), "pine")
	if err := os.WriteFile(other, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if installHint(Options{Executable: other}) != "release/binary install" {
		t.Fatal(installHint(Options{Executable: other}))
	}
}

func TestOptionsClientDefault(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	opt := Options{CurrentVersion: "0.1.0"}
	c := opt.client()
	if c.Version != "0.1.0" {
		t.Fatal(c.Version)
	}
	if opt.goos() == "" || opt.goarch() == "" {
		t.Fatal("runtime defaults")
	}
}

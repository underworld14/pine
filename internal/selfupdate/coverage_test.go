package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestUpgradeErrorPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("tar.gz mock")
	}
	ctx := context.Background()
	archiveName := "pine_0.9.0_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"
	payload := []byte("new-bin")
	archive := mustTarGz(t, "pine", payload)
	sum := sha256.Sum256(archive)
	goodSum := hex.EncodeToString(sum[:]) + "  " + archiveName + "\n"

	t.Run("missing platform asset", func(t *testing.T) {
		var serverURL string
		mux := http.NewServeMux()
		mux.HandleFunc("/repos/underworld14/pine/releases/latest", func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(Release{
				TagName: "v0.9.0",
				Assets:  []Asset{{Name: "checksums.txt", BrowserDownloadURL: serverURL + "/c"}},
			})
		})
		ts := httptest.NewServer(mux)
		defer ts.Close()
		serverURL = ts.URL
		_, err := Upgrade(ctx, Options{
			CurrentVersion: "0.8.0",
			GOOS:           runtime.GOOS,
			GOARCH:         runtime.GOARCH,
			Client:         &Client{HTTP: ts.Client(), APIBase: ts.URL, Owner: DefaultOwner, Repo: DefaultRepo},
		})
		if err == nil {
			t.Fatal("expected missing asset")
		}
	})

	t.Run("missing checksums", func(t *testing.T) {
		var serverURL string
		mux := http.NewServeMux()
		mux.HandleFunc("/repos/underworld14/pine/releases/latest", func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(Release{
				TagName: "v0.9.0",
				Assets:  []Asset{{Name: archiveName, BrowserDownloadURL: serverURL + "/a"}},
			})
		})
		ts := httptest.NewServer(mux)
		defer ts.Close()
		serverURL = ts.URL
		_, err := Upgrade(ctx, Options{
			CurrentVersion: "0.8.0",
			GOOS:           runtime.GOOS,
			GOARCH:         runtime.GOARCH,
			Client:         &Client{HTTP: ts.Client(), APIBase: ts.URL, Owner: DefaultOwner, Repo: DefaultRepo},
			Executable:     filepath.Join(t.TempDir(), "pine"),
		})
		if err == nil || !strings.Contains(err.Error(), "checksums.txt") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("checksum mismatch", func(t *testing.T) {
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
		mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(archive) })
		mux.HandleFunc("/c", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("0000000000000000000000000000000000000000000000000000000000000000  " + archiveName + "\n"))
		})
		ts := httptest.NewServer(mux)
		defer ts.Close()
		serverURL = ts.URL
		exe := filepath.Join(t.TempDir(), "pine")
		_ = os.WriteFile(exe, []byte("old"), 0o755)
		_, err := Upgrade(ctx, Options{
			CurrentVersion: "0.8.0",
			GOOS:           runtime.GOOS,
			GOARCH:         runtime.GOARCH,
			Client:         &Client{HTTP: ts.Client(), APIBase: ts.URL, Owner: DefaultOwner, Repo: DefaultRepo},
			Executable:     exe,
		})
		if err == nil || !strings.Contains(err.Error(), "checksum") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("wrong binary in archive", func(t *testing.T) {
		badArchive := mustTarGz(t, "not-pine", payload)
		badSum := sha256.Sum256(badArchive)
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
		mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(badArchive) })
		mux.HandleFunc("/c", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(hex.EncodeToString(badSum[:]) + "  " + archiveName + "\n"))
		})
		ts := httptest.NewServer(mux)
		defer ts.Close()
		serverURL = ts.URL
		exe := filepath.Join(t.TempDir(), "pine")
		_ = os.WriteFile(exe, []byte("old"), 0o755)
		_, err := Upgrade(ctx, Options{
			CurrentVersion: "0.8.0",
			GOOS:           runtime.GOOS,
			GOARCH:         runtime.GOARCH,
			Client:         &Client{HTTP: ts.Client(), APIBase: ts.URL, Owner: DefaultOwner, Repo: DefaultRepo},
			Executable:     exe,
		})
		if err == nil {
			t.Fatal("expected extract error")
		}
	})

	t.Run("installHint gobin", func(t *testing.T) {
		gobin := t.TempDir()
		t.Setenv("GOBIN", gobin)
		exe := filepath.Join(gobin, "pine")
		_ = os.WriteFile(exe, []byte("x"), 0o755)
		hint := installHint(Options{Executable: exe})
		if !strings.Contains(hint, "go") {
			t.Fatalf("hint=%q", hint)
		}
		_ = goodSum
	})
}

func TestReplaceViaMoveAsideRollback(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "pine")
	if err := os.WriteFile(path, []byte("v1"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Missing tmp → install rename fails after move-aside; current restored.
	err := replaceViaMoveAside(path, filepath.Join(dir, "missing-tmp"))
	if err == nil {
		t.Fatal("expected error")
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil || string(got) != "v1" {
		t.Fatalf("restored=%q err=%v", got, readErr)
	}
}

func TestChecksumForEdgeCases(t *testing.T) {
	t.Parallel()
	body := "\nshort  file\nabc  skip\n" + strings.Repeat("a", 64) + "  pine_1.0.0_linux_amd64.tar.gz\n"
	sum, err := ChecksumFor(body, "pine_1.0.0_linux_amd64.tar.gz")
	if err != nil || sum != strings.Repeat("a", 64) {
		t.Fatalf("sum=%q err=%v", sum, err)
	}
	if _, err := ChecksumFor("abcd  pine.tar.gz\n", "pine.tar.gz"); err == nil {
		t.Fatal("invalid length")
	}
	if _, err := ChecksumFor("", "x"); err == nil {
		t.Fatal("missing")
	}
}

func TestExtractZipNestedAndMiss(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("dist/pine.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("win")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := ExtractBinary(buf.Bytes(), "pine.zip", "windows")
	if err != nil || string(got) != "win" {
		t.Fatalf("got=%q err=%v", got, err)
	}
	var buf2 bytes.Buffer
	zw2 := zip.NewWriter(&buf2)
	w2, _ := zw2.Create("readme.txt")
	_, _ = w2.Write([]byte("docs"))
	_ = zw2.Close()
	if _, err := ExtractBinary(buf2.Bytes(), "pine.zip", "windows"); err == nil {
		t.Fatal("expected miss")
	}
}

func TestInstallHintGoBinSubstring(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "extra", "go", "bin", "nested")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "pine")
	if err := os.WriteFile(exe, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOBIN", "")
	t.Setenv("GOPATH", filepath.Join(t.TempDir(), "elsewhere"))
	hint := installHint(Options{Executable: exe})
	if !strings.Contains(hint, "Go bin") {
		t.Fatalf("hint=%q", hint)
	}
}

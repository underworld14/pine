package server

import (
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/underworld14/pine/web"
)

// TestHandleStaticPlaceholder confirms the dev-placeholder fallback: the test
// binary is built without -tags embedassets, so web.HasAssets() is always
// false and handleStatic must serve the "Pine is running" placeholder for any
// path, never the embedded-SPA branch.
func TestHandleStaticPlaceholder(t *testing.T) {
	ts := newTestServer(t)

	for _, path := range []string{"/", "/some/client/route"} {
		resp, body := do(t, "GET", ts.URL+path, "", nil)
		if resp.StatusCode != 200 {
			t.Fatalf("%s: status %d", path, resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
			t.Errorf("%s: content-type = %q", path, ct)
		}
		if !strings.Contains(body, "Pine is running") {
			t.Errorf("%s: expected placeholder body, got: %s", path, body)
		}
	}
}

// TestServeAsset exercises serveAsset directly against a fake embedded
// filesystem (web.Assets is a package-level var we can substitute in a test;
// handleStatic itself cannot reach this branch here because web.HasAssets()
// is hardcoded false in a non-embedassets build).
func TestServeAsset(t *testing.T) {
	orig := web.Assets
	web.Assets = fstest.MapFS{
		"index.html":            &fstest.MapFile{Data: []byte("<html>hi</html>")},
		"_app/immutable/foo.js": &fstest.MapFile{Data: []byte("console.log(1)")},
	}
	t.Cleanup(func() { web.Assets = orig })

	// Existing top-level asset: short cache.
	req := httptest.NewRequest("GET", "/index.html", nil)
	rw := httptest.NewRecorder()
	if !serveAsset(rw, req, "index.html") {
		t.Fatal("expected serveAsset to find index.html")
	}
	if got := rw.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("cache-control = %q, want no-cache", got)
	}
	if rw.Body.String() != "<html>hi</html>" {
		t.Errorf("body = %q", rw.Body.String())
	}

	// _app/-prefixed asset: long-lived immutable cache.
	req2 := httptest.NewRequest("GET", "/_app/immutable/foo.js", nil)
	rw2 := httptest.NewRecorder()
	if !serveAsset(rw2, req2, "_app/immutable/foo.js") {
		t.Fatal("expected serveAsset to find the _app asset")
	}
	if got := rw2.Header().Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Errorf("cache-control = %q, want immutable", got)
	}

	// Missing file.
	req3 := httptest.NewRequest("GET", "/missing.txt", nil)
	rw3 := httptest.NewRecorder()
	if serveAsset(rw3, req3, "missing.txt") {
		t.Fatal("expected serveAsset to report missing file")
	}

	// A directory path (implied by "_app/immutable/foo.js") must not be served
	// as a file.
	req4 := httptest.NewRequest("GET", "/_app", nil)
	rw4 := httptest.NewRecorder()
	if serveAsset(rw4, req4, "_app") {
		t.Fatal("expected serveAsset to reject a directory path")
	}
}

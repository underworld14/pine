package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/underworld14/pine/internal/config"
	"github.com/underworld14/pine/internal/store"
	"github.com/underworld14/pine/internal/view"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	pine := filepath.Join(t.TempDir(), ".pine")
	if err := os.MkdirAll(filepath.Join(pine, "tickets"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default("test")
	cfg.IDStyle = "sequential"
	cfgB, _ := cfg.Bytes()
	os.WriteFile(filepath.Join(pine, "config.json"), cfgB, 0o644)
	bB, _ := config.DefaultBoard().Bytes()
	os.WriteFile(filepath.Join(pine, "board.json"), bB, 0o644)
	st, err := store.Open(pine)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(New(st, "test").Handler())
	t.Cleanup(ts.Close)
	return ts
}

func do(t *testing.T, method, url, body string, headers map[string]string) (*http.Response, string) {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, string(data)
}

func TestHealth(t *testing.T) {
	ts := newTestServer(t)
	resp, body := do(t, "GET", ts.URL+"/api/health", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if !strings.Contains(body, `"ok":true`) || !strings.Contains(body, `"project":"test"`) {
		t.Errorf("body = %s", body)
	}
}

func TestTicketCRUDAndIfMatch(t *testing.T) {
	ts := newTestServer(t)

	// Create.
	resp, body := do(t, "POST", ts.URL+"/api/tickets", `{"type":"bug","title":"X","priority":"high"}`, nil)
	if resp.StatusCode != 201 {
		t.Fatalf("create status %d: %s", resp.StatusCode, body)
	}
	var created view.Ticket
	json.Unmarshal([]byte(body), &created)
	if created.ID != "BUG-001" {
		t.Fatalf("id = %s", created.ID)
	}

	// Get returns an ETag.
	resp, body = do(t, "GET", ts.URL+"/api/tickets/BUG-001", "", nil)
	etag := resp.Header.Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag")
	}

	// PATCH with a wrong If-Match → 409 with current.
	resp, body = do(t, "PATCH", ts.URL+"/api/tickets/BUG-001", `{"status":"doing"}`,
		map[string]string{"If-Match": `"deadbeef"`})
	if resp.StatusCode != 409 {
		t.Fatalf("expected 409, got %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"current"`) {
		t.Errorf("409 body should include current: %s", body)
	}

	// PATCH with the correct If-Match → 200.
	resp, body = do(t, "PATCH", ts.URL+"/api/tickets/BUG-001", `{"status":"doing"}`,
		map[string]string{"If-Match": etag})
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	var updated view.Ticket
	json.Unmarshal([]byte(body), &updated)
	if updated.Status != "doing" {
		t.Errorf("status = %s", updated.Status)
	}

	// Delete → 204, then Get → 404.
	resp, _ = do(t, "DELETE", ts.URL+"/api/tickets/BUG-001", "", nil)
	if resp.StatusCode != 204 {
		t.Fatalf("delete status %d", resp.StatusCode)
	}
	resp, _ = do(t, "GET", ts.URL+"/api/tickets/BUG-001", "", nil)
	if resp.StatusCode != 404 {
		t.Fatalf("get after delete status %d", resp.StatusCode)
	}
}

func TestCreateUnknownType422(t *testing.T) {
	ts := newTestServer(t)
	resp, body := do(t, "POST", ts.URL+"/api/tickets", `{"type":"zzz","title":"x"}`, nil)
	if resp.StatusCode != 422 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
}

func TestSnapshotAndBoard(t *testing.T) {
	ts := newTestServer(t)
	do(t, "POST", ts.URL+"/api/tickets", `{"type":"bug","title":"x"}`, nil)
	do(t, "POST", ts.URL+"/api/tickets", `{"type":"feature","title":"y","status":"weird"}`, nil)

	_, body := do(t, "GET", ts.URL+"/api/snapshot", "", nil)
	var snap struct {
		Tickets []view.Ticket `json:"tickets"`
		Board   boardResp     `json:"board"`
	}
	json.Unmarshal([]byte(body), &snap)
	if len(snap.Tickets) != 2 {
		t.Fatalf("tickets = %d", len(snap.Tickets))
	}
	// "weird" status is not a board column → surfaced as unmapped.
	found := false
	for _, u := range snap.Board.Unmapped {
		if u == "weird" {
			found = true
		}
	}
	if !found {
		t.Errorf("unmapped should include weird: %+v", snap.Board.Unmapped)
	}
}

func TestSecurityHostAndOrigin(t *testing.T) {
	ts := newTestServer(t)

	// Bad Host header → 403.
	req, _ := http.NewRequest("GET", ts.URL+"/api/health", nil)
	req.Host = "evil.example.com"
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != 403 {
		t.Errorf("bad host should be 403, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Cross-origin POST → 403.
	resp, _ = do(t, "POST", ts.URL+"/api/tickets", `{"type":"bug","title":"x"}`,
		map[string]string{"Origin": "http://evil.example.com"})
	if resp.StatusCode != 403 {
		t.Errorf("cross-origin POST should be 403, got %d", resp.StatusCode)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	ts := newTestServer(t)
	_, body := do(t, "GET", ts.URL+"/api/config", "", nil)
	if !strings.Contains(body, `"attachments"`) {
		t.Fatalf("config body = %s", body)
	}
	// PUT an invalid config → 422.
	resp, _ := do(t, "PUT", ts.URL+"/api/config", `{"version":1,"git":{"backend":"svn"},"types":[],"priorities":[],"attachments":{"quality":80,"maxDimension":2000}}`, nil)
	if resp.StatusCode != 422 {
		t.Errorf("invalid config should be 422, got %d", resp.StatusCode)
	}
}

func TestPutConfigSuccess(t *testing.T) {
	ts := newTestServer(t)
	resp, body := do(t, "PUT", ts.URL+"/api/config", `{"crossBranch":{"enabled":false,"activeBranchDays":7}}`, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"activeBranchDays":7`) {
		t.Errorf("update should apply partial field: %s", body)
	}
	// The rest of the config (e.g. project name) must be preserved, not reset.
	if !strings.Contains(body, `"name":"test"`) {
		t.Errorf("unrelated fields should be preserved: %s", body)
	}
}

func TestPutConfigMalformedJSON400(t *testing.T) {
	ts := newTestServer(t)
	resp, body := do(t, "PUT", ts.URL+"/api/config", `{bad json`, nil)
	if resp.StatusCode != 400 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "bad_request") {
		t.Errorf("expected bad_request code: %s", body)
	}
}

func TestHandleBoard(t *testing.T) {
	ts := newTestServer(t)
	do(t, "POST", ts.URL+"/api/tickets", `{"type":"bug","title":"x","status":"weird"}`, nil)

	resp, body := do(t, "GET", ts.URL+"/api/board", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	var b boardResp
	if err := json.Unmarshal([]byte(body), &b); err != nil {
		t.Fatalf("bad json: %v: %s", err, body)
	}
	if len(b.Columns) == 0 {
		t.Errorf("expected default board columns: %+v", b)
	}
	found := false
	for _, u := range b.Unmapped {
		if u == "weird" {
			found = true
		}
	}
	if !found {
		t.Errorf("unmapped should include weird: %+v", b.Unmapped)
	}
}

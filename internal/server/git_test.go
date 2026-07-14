package server

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/underworld14/pine/internal/gitx"
)

func TestHandleGitNonRepo(t *testing.T) {
	ts := newTestServer(t)
	resp, body := do(t, "GET", ts.URL+"/api/git", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	var s gitx.Status
	if err := json.Unmarshal([]byte(body), &s); err != nil {
		t.Fatalf("bad json: %v: %s", err, body)
	}
	if s.IsRepo {
		t.Errorf("expected not a repo: %+v", s)
	}
}

func TestHandleGitRealRepo(t *testing.T) {
	ts := newCrossBranchServer(t, "hash")
	resp, body := do(t, "GET", ts.URL+"/api/git", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	var s gitx.Status
	if err := json.Unmarshal([]byte(body), &s); err != nil {
		t.Fatalf("bad json: %v: %s", err, body)
	}
	if !s.IsRepo {
		t.Errorf("expected a repo: %+v", s)
	}
	if s.Branch != "main" {
		t.Errorf("branch = %q, want main", s.Branch)
	}
}

func TestHandleFiles(t *testing.T) {
	ts := newCrossBranchServer(t, "hash")

	resp, body := do(t, "GET", ts.URL+"/api/files", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	var out struct {
		Items []FileItem `json:"items"`
		Files []string   `json:"files"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("bad json: %v: %s", err, body)
	}
	found := false
	for _, it := range out.Items {
		if strings.Contains(it.Path, "BUG-0a1b2c.md") && it.Kind == "file" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the tracked ticket file among items: %+v", out.Items)
	}
	if len(out.Files) == 0 {
		t.Fatalf("expected files alias populated")
	}

	// Filtered query — items include matching paths.
	_, body2 := do(t, "GET", ts.URL+"/api/files?q=bug-0a1b2c", "", nil)
	var out2 struct {
		Items []FileItem `json:"items"`
		Files []string   `json:"files"`
	}
	json.Unmarshal([]byte(body2), &out2)
	if len(out2.Items) == 0 {
		t.Fatalf("expected a match for filtered query: %s", body2)
	}
	for _, it := range out2.Items {
		if !strings.Contains(strings.ToLower(it.Path), "bug-0a1b2c") {
			t.Errorf("unexpected item in filtered results: %+v", it)
		}
	}

	// Query matching nothing.
	_, body3 := do(t, "GET", ts.URL+"/api/files?q=zzz-does-not-exist", "", nil)
	var out3 struct {
		Items []FileItem `json:"items"`
		Files []string   `json:"files"`
	}
	json.Unmarshal([]byte(body3), &out3)
	if len(out3.Items) != 0 || len(out3.Files) != 0 {
		t.Errorf("expected no matches: items=%+v files=%+v", out3.Items, out3.Files)
	}
}

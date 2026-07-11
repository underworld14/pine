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
		Files []string `json:"files"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("bad json: %v: %s", err, body)
	}
	found := false
	for _, f := range out.Files {
		if strings.Contains(f, "BUG-0a1b2c.md") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the tracked ticket file among files: %+v", out.Files)
	}

	// Filtered query.
	_, body2 := do(t, "GET", ts.URL+"/api/files?q=bug-0a1b2c", "", nil)
	var out2 struct {
		Files []string `json:"files"`
	}
	json.Unmarshal([]byte(body2), &out2)
	if len(out2.Files) == 0 {
		t.Fatalf("expected a match for filtered query: %s", body2)
	}
	for _, f := range out2.Files {
		if !strings.Contains(strings.ToLower(f), "bug-0a1b2c") {
			t.Errorf("unexpected file in filtered results: %s", f)
		}
	}

	// Query matching nothing.
	_, body3 := do(t, "GET", ts.URL+"/api/files?q=zzz-does-not-exist", "", nil)
	var out3 struct {
		Files []string `json:"files"`
	}
	json.Unmarshal([]byte(body3), &out3)
	if len(out3.Files) != 0 {
		t.Errorf("expected no matches: %+v", out3.Files)
	}
}

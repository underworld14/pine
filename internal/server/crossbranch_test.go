package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/underworld14/pine/internal/config"
	"github.com/underworld14/pine/internal/store"
	"github.com/underworld14/pine/internal/view"
)

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=t@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func ticketMD(id, status, updated string) string {
	return "---\nid: " + id + "\ntitle: " + id + "\nstatus: " + status + "\nupdated: " + updated + "\n---\n\nbody\n"
}

func writeTicketFile(t *testing.T, repo, id, content string) {
	t.Helper()
	p := filepath.Join(repo, ".pine", "tickets", id+".md")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newCrossBranchServer builds a server over a real git repo where BUG-0a1b2c
// lives on main (local/editable) and FEAT-3d4e5f lives only on a feature branch
// (off-branch). idStyle selects hash (aggregates) vs sequential (must not).
func newCrossBranchServer(t *testing.T, idStyle string) *httptest.Server {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	repo := t.TempDir()
	gitRun(t, repo, "init", "-q")
	gitRun(t, repo, "checkout", "-b", "main")

	pine := filepath.Join(repo, ".pine")
	if err := os.MkdirAll(filepath.Join(pine, "tickets"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default("test")
	cfg.IDStyle = idStyle
	cfgB, _ := cfg.Bytes()
	os.WriteFile(filepath.Join(pine, "config.json"), cfgB, 0o644)
	bB, _ := config.DefaultBoard().Bytes()
	os.WriteFile(filepath.Join(pine, "board.json"), bB, 0o644)

	// Ticket on main (present in the working tree).
	writeTicketFile(t, repo, "BUG-0a1b2c", ticketMD("BUG-0a1b2c", "todo", "2026-07-01T10:00:00Z"))
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-q", "-m", "main ticket")

	// Ticket only on the feature branch (off-branch once we return to main).
	gitRun(t, repo, "checkout", "-q", "-b", "feature")
	writeTicketFile(t, repo, "FEAT-3d4e5f", ticketMD("FEAT-3d4e5f", "doing", "2026-07-02T10:00:00Z"))
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-q", "-m", "feature ticket")
	gitRun(t, repo, "checkout", "-q", "main") // working tree no longer has FEAT

	st, err := store.Open(pine)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(New(st, "test").Handler())
	t.Cleanup(ts.Close)
	return ts
}

func snapshotTickets(t *testing.T, body string) []view.Ticket {
	t.Helper()
	var snap struct {
		Tickets []view.Ticket `json:"tickets"`
	}
	if err := json.Unmarshal([]byte(body), &snap); err != nil {
		t.Fatalf("bad snapshot json: %v", err)
	}
	return snap.Tickets
}

func TestSnapshotIncludesOffBranchTicket(t *testing.T) {
	ts := newCrossBranchServer(t, "hash")
	resp, body := do(t, "GET", ts.URL+"/api/snapshot", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	tickets := snapshotTickets(t, body)

	var feat, bug *view.Ticket
	for i := range tickets {
		switch tickets[i].ID {
		case "FEAT-3d4e5f":
			feat = &tickets[i]
		case "BUG-0a1b2c":
			bug = &tickets[i]
		}
	}
	if feat == nil {
		t.Fatalf("off-branch ticket missing from snapshot: %s", body)
	}
	if feat.Source != "local-branch" || feat.Branch != "feature" || !feat.ReadOnly {
		t.Errorf("off-branch fields wrong: source=%q branch=%q readOnly=%v", feat.Source, feat.Branch, feat.ReadOnly)
	}
	if bug == nil {
		t.Fatalf("local ticket missing from snapshot")
	}
	if bug.Source != "local" || bug.ReadOnly {
		t.Errorf("local ticket should be editable: source=%q readOnly=%v", bug.Source, bug.ReadOnly)
	}
}

func TestOffBranchWritesRejected(t *testing.T) {
	ts := newCrossBranchServer(t, "hash")
	hdr := map[string]string{"Content-Type": "application/json"}

	resp, body := do(t, "PATCH", ts.URL+"/api/tickets/FEAT-3d4e5f", `{"status":"done"}`, hdr)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("patch off-branch: expected 409, got %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "off_branch") {
		t.Errorf("expected off_branch code: %s", body)
	}

	resp2, _ := do(t, "DELETE", ts.URL+"/api/tickets/FEAT-3d4e5f", "", nil)
	if resp2.StatusCode != http.StatusConflict {
		t.Errorf("delete off-branch: expected 409, got %d", resp2.StatusCode)
	}
}

func TestSequentialRepoDoesNotAggregate(t *testing.T) {
	ts := newCrossBranchServer(t, "sequential")
	_, body := do(t, "GET", ts.URL+"/api/snapshot", "", nil)
	for _, tk := range snapshotTickets(t, body) {
		if tk.ID == "FEAT-3d4e5f" {
			t.Fatalf("sequential repo must not aggregate off-branch tickets: %s", body)
		}
	}
}

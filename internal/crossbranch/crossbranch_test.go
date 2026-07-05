package crossbranch

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/underworld14/pine/internal/gitx"
)

// stubGit is an in-memory gitx.Client for deterministic tests.
type stubGit struct {
	branches []gitx.Branch
	trees    map[string]map[string]string // sha -> path -> content
}

func (s *stubGit) IsRepo(context.Context) bool               { return true }
func (s *stubGit) Snapshot(context.Context, int) gitx.Status { return gitx.Status{} }
func (s *stubGit) Files(context.Context) []string            { return nil }
func (s *stubGit) Branches(context.Context) []gitx.Branch    { return s.branches }

func (s *stubGit) ListTreeFiles(_ context.Context, rev, pathspec string) []string {
	var out []string
	for p := range s.trees[rev] {
		if p == pathspec || strings.HasPrefix(p, pathspec+"/") {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

func (s *stubGit) ShowFile(_ context.Context, rev, p string) ([]byte, bool) {
	if c, ok := s.trees[rev][p]; ok {
		return []byte(c), true
	}
	return nil, false
}

func mkTicket(id, status, updated string) string {
	return "---\nid: " + id + "\ntitle: " + id + "\nstatus: " + status + "\nupdated: " + updated + "\n---\n\nbody\n"
}

func baseOpts(now time.Time) Options {
	return Options{
		Enabled:     true,
		IDStyle:     "hash",
		ActiveDays:  30,
		TicketsPath: ".pine/tickets",
		Now:         func() time.Time { return now },
	}
}

func TestComputeBasicOffBranchExclusionAndCurrentSkip(t *testing.T) {
	now := time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC)
	g := &stubGit{
		branches: []gitx.Branch{
			{Name: "main", SHA: "sha-main", CommitDate: now},                      // current — must be skipped
			{Name: "feature", SHA: "sha-feat", CommitDate: now.AddDate(0, 0, -1)}, // off branch
		},
		trees: map[string]map[string]string{
			"sha-feat": {
				".pine/tickets/BUG-0a1b2c.md":  mkTicket("BUG-0a1b2c", "todo", "2026-07-01T10:00:00Z"),
				".pine/tickets/FEAT-3d4e5f.md": mkTicket("FEAT-3d4e5f", "doing", "2026-07-02T10:00:00Z"),
			},
		},
	}
	// BUG is already in the working tree; only FEAT is genuinely off-branch.
	localIDs := map[string]bool{"BUG-0a1b2c": true}

	got := Compute(context.Background(), g, "main", localIDs, baseOpts(now))
	if len(got) != 1 {
		t.Fatalf("got %d off-branch tickets, want 1: %+v", len(got), got)
	}
	if got[0].Ticket.ID != "FEAT-3d4e5f" || got[0].Branch != "feature" {
		t.Errorf("got %s from %s, want FEAT-3d4e5f from feature", got[0].Ticket.ID, got[0].Branch)
	}
}

func TestComputeNewestUpdatedWins(t *testing.T) {
	now := time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC)
	g := &stubGit{
		branches: []gitx.Branch{
			{Name: "feat-a", SHA: "sha-a", CommitDate: now.AddDate(0, 0, -2)},
			{Name: "feat-b", SHA: "sha-b", CommitDate: now.AddDate(0, 0, -1)},
		},
		trees: map[string]map[string]string{
			"sha-a": {".pine/tickets/FEAT-9z8y7x.md": mkTicket("FEAT-9z8y7x", "todo", "2026-07-01T10:00:00Z")},
			"sha-b": {".pine/tickets/FEAT-9z8y7x.md": mkTicket("FEAT-9z8y7x", "doing", "2026-07-03T10:00:00Z")},
		},
	}
	got := Compute(context.Background(), g, "main", nil, baseOpts(now))
	if len(got) != 1 {
		t.Fatalf("got %d, want 1 (deduped by ID): %+v", len(got), got)
	}
	if got[0].Branch != "feat-b" || got[0].Ticket.Status != "doing" {
		t.Errorf("winner = %s/%s, want feat-b/doing (newest updated)", got[0].Branch, got[0].Ticket.Status)
	}
}

func TestComputeSequentialIDStyleReturnsNothing(t *testing.T) {
	now := time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC)
	g := &stubGit{
		branches: []gitx.Branch{{Name: "feature", SHA: "sha-feat", CommitDate: now}},
		trees: map[string]map[string]string{
			"sha-feat": {".pine/tickets/BUG-3d4e5f.md": mkTicket("BUG-3d4e5f", "todo", "2026-07-02T10:00:00Z")},
		},
	}
	opts := baseOpts(now)
	opts.IDStyle = "sequential"
	if got := Compute(context.Background(), g, "main", nil, opts); got != nil {
		t.Errorf("sequential IDStyle must aggregate nothing, got %+v", got)
	}
}

func TestComputeDisabledReturnsNothing(t *testing.T) {
	now := time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC)
	g := &stubGit{
		branches: []gitx.Branch{{Name: "feature", SHA: "sha-feat", CommitDate: now}},
		trees: map[string]map[string]string{
			"sha-feat": {".pine/tickets/FEAT-3d4e5f.md": mkTicket("FEAT-3d4e5f", "todo", "2026-07-02T10:00:00Z")},
		},
	}
	opts := baseOpts(now)
	opts.Enabled = false
	if got := Compute(context.Background(), g, "main", nil, opts); got != nil {
		t.Errorf("disabled must aggregate nothing, got %+v", got)
	}
}

func TestComputeRecencyWindowFiltersOldBranches(t *testing.T) {
	now := time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC)
	g := &stubGit{
		branches: []gitx.Branch{
			{Name: "recent", SHA: "sha-r", CommitDate: now.AddDate(0, 0, -5)},
			{Name: "stale", SHA: "sha-s", CommitDate: now.AddDate(0, 0, -60)}, // outside 30-day window
		},
		trees: map[string]map[string]string{
			"sha-r": {".pine/tickets/FEAT-111111.md": mkTicket("FEAT-111111", "todo", "2026-07-01T10:00:00Z")},
			"sha-s": {".pine/tickets/FEAT-222222.md": mkTicket("FEAT-222222", "todo", "2026-05-01T10:00:00Z")},
		},
	}
	got := Compute(context.Background(), g, "main", nil, baseOpts(now))
	if len(got) != 1 || got[0].Ticket.ID != "FEAT-111111" {
		t.Errorf("recency window not applied: %+v", got)
	}
}

func TestComputeSkipsSequentialFormIDsEvenUnderHashConfig(t *testing.T) {
	// Simulates a legacy repo whose stale config still reports idStyle "hash"
	// but whose tickets are sequential: those IDs collide across branches and
	// must never be aggregated, regardless of the config gate.
	now := time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC)
	g := &stubGit{
		branches: []gitx.Branch{{Name: "feature", SHA: "sha-feat", CommitDate: now}},
		trees: map[string]map[string]string{
			"sha-feat": {
				".pine/tickets/BUG-042.md":     mkTicket("BUG-042", "todo", "2026-07-02T10:00:00Z"),      // sequential → skip
				".pine/tickets/FEAT-3d4e5f.md": mkTicket("FEAT-3d4e5f", "doing", "2026-07-02T10:00:00Z"), // hash → keep
			},
		},
	}
	got := Compute(context.Background(), g, "main", nil, baseOpts(now))
	if len(got) != 1 || got[0].Ticket.ID != "FEAT-3d4e5f" {
		t.Fatalf("sequential-form IDs must be skipped, only hash aggregated: %+v", got)
	}
}

func TestComputeFallsBackToBranchDateWhenUpdatedMissing(t *testing.T) {
	now := time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC)
	// Ticket with no frontmatter `updated`; recency/display must fall back to the
	// branch commit date rather than the zero time.
	noUpdated := "---\nid: FEAT-3d4e5f\ntitle: FEAT-3d4e5f\nstatus: todo\n---\n\nbody\n"
	branchDate := now.AddDate(0, 0, -3)
	g := &stubGit{
		branches: []gitx.Branch{{Name: "feature", SHA: "sha-feat", CommitDate: branchDate}},
		trees: map[string]map[string]string{
			"sha-feat": {".pine/tickets/FEAT-3d4e5f.md": noUpdated},
		},
	}
	got := Compute(context.Background(), g, "main", nil, baseOpts(now))
	if len(got) != 1 {
		t.Fatalf("expected 1 off-branch ticket, got %+v", got)
	}
	if !got[0].Ticket.Updated.Equal(branchDate) {
		t.Errorf("Updated fallback = %v, want branch date %v", got[0].Ticket.Updated, branchDate)
	}
}

func TestComputeIgnoresNonTicketFilesAndInvalidIDs(t *testing.T) {
	now := time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC)
	g := &stubGit{
		branches: []gitx.Branch{{Name: "feature", SHA: "sha-feat", CommitDate: now}},
		trees: map[string]map[string]string{
			"sha-feat": {
				".pine/tickets/FEAT-3d4e5f.md": mkTicket("FEAT-3d4e5f", "todo", "2026-07-02T10:00:00Z"),
				".pine/tickets/README.md":      "not a ticket\n", // invalid id → ignored
				".pine/tickets/notes.txt":      "scratch\n",      // not .md → ignored
			},
		},
	}
	got := Compute(context.Background(), g, "main", nil, baseOpts(now))
	if len(got) != 1 || got[0].Ticket.ID != "FEAT-3d4e5f" {
		t.Errorf("should only pick the valid ticket: %+v", got)
	}
}

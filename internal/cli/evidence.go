package cli

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/underworld14/pine/internal/gitx"
	"github.com/underworld14/pine/internal/store"
)

// Limits for running a ticket's `verify:` proof command.
const (
	verifyTimeout    = 2 * time.Minute
	verifyOutputTail = 40 // lines of combined output kept in the ticket
)

// workEvidence computes a markdown "## Work Evidence" block summarizing the
// file changes attributable to a ticket: commits that reference or touch it,
// plus a `git diff --stat` from the commit at the ticket's creation time to the
// working tree. Best-effort: degrades to a note when git is absent or the
// ticket's creation time is unknown.
//
// The git work runs OUTSIDE the store lock (this is called before s.Update),
// so the only locked step is the body mutation in the caller.
func workEvidence(s *store.Store, id string) string {
	client := gitx.New(filepath.Dir(s.Root()))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var b strings.Builder
	b.WriteString("## Work Evidence\n\n")
	b.WriteString(fmt.Sprintf("Closed by `pine close --evidence` on %s.\n\n", time.Now().UTC().Format("2006-01-02")))

	if !client.IsRepo(ctx) {
		b.WriteString("_(not a git repository — no file-change evidence available)_\n")
		return b.String()
	}

	t, err := s.Get(id)
	if err != nil {
		b.WriteString("_(ticket not found — no file-change evidence available)_\n")
		return b.String()
	}

	base := client.CommitBefore(ctx, t.Created)
	pathspec := path.Join(filepath.Base(s.Root()), "tickets", id+".md")
	commits := client.Log(ctx, pathspec, id, 50)
	stat := client.DiffStat(ctx, base)

	if base != "" {
		b.WriteString(fmt.Sprintf("- Base: `%s` (last commit at or before ticket created %s)\n", shortSHA(base), t.Created.Format("2006-01-02")))
	} else {
		b.WriteString("- Base: _(none — ticket predates git history or creation time unknown; showing uncommitted changes only)_\n")
	}

	hasCommits := len(commits) > 0
	hasStat := strings.TrimSpace(stat) != ""

	if hasCommits {
		writeCommitList(&b, commits)
	}
	if hasStat {
		b.WriteString("- Files changed (base → working tree):\n\n```\n")
		b.WriteString(stat)
		b.WriteString("\n```\n")
	}
	if !hasCommits && !hasStat {
		b.WriteString("- _(no file changes detected since ticket creation)_\n")
	}
	return b.String()
}

// writeCommitList renders the "- Commits (n):" bullets used by both evidence
// modes, so recorded and auto-derived evidence read the same.
func writeCommitList(b *strings.Builder, commits []gitx.Commit) {
	fmt.Fprintf(b, "- Commits (%d):\n", len(commits))
	for _, c := range commits {
		fmt.Fprintf(b, "  - `%s` — %s\n", c.Hash, c.Subject)
	}
}

// recordedEvidenceMarker opens the line of a Work Evidence section written from
// explicit commits (pine create/close --commit). pine close --evidence leaves
// such a section alone: re-deriving it from the ticket's creation time would
// replace exact evidence with a diff of every unrelated change since.
const recordedEvidenceMarker = "Recorded from commits by"

// recordedCommits are commits the user named with --commit, resolved up front
// so a bad rev fails before any ticket is created or closed.
type recordedCommits struct {
	client  gitx.Client
	commits []gitx.Commit
	shas    []string
}

func resolveCommits(s *store.Store, revs []string) (*recordedCommits, error) {
	client := gitx.New(filepath.Dir(s.Root()))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if !client.IsRepo(ctx) {
		return nil, errors.New("--commit needs a git repository")
	}
	r := &recordedCommits{client: client}
	for _, rev := range revs {
		sha, c, ok := client.ResolveCommit(ctx, rev)
		if !ok {
			return nil, fmt.Errorf("unknown commit %q", rev)
		}
		r.shas = append(r.shas, sha)
		r.commits = append(r.commits, c)
	}
	return r, nil
}

// evidence renders the "## Work Evidence" section for exactly these commits:
// the commit bullets plus a capped per-commit file stat. by names the command
// that recorded it.
func (r *recordedCommits) evidence(by string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var b strings.Builder
	b.WriteString("## Work Evidence\n\n")
	fmt.Fprintf(&b, "%s `%s` on %s.\n\n", recordedEvidenceMarker, by, time.Now().UTC().Format("2006-01-02"))
	writeCommitList(&b, r.commits)
	if stat := r.client.CommitStat(ctx, r.shas); strings.TrimSpace(stat) != "" {
		b.WriteString("- Files changed:\n\n```\n")
		b.WriteString(stat)
		b.WriteString("\n```\n")
	}
	return b.String()
}

// hasRecordedEvidence reports whether the body's Work Evidence section was
// recorded from explicit commits (see recordedEvidenceMarker).
func hasRecordedEvidence(body string) bool {
	start, end, ok := sectionBounds(body, "Work Evidence")
	return ok && strings.Contains(body[start:end], recordedEvidenceMarker)
}

func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

// verifyResult is the outcome of running a ticket's frontmatter `verify:`
// command. ran is false when the ticket defines no command.
type verifyResult struct {
	command  string
	ran      bool
	exitCode int
	err      error // non-nil on non-zero exit, signal, or timeout
	output   string
	duration time.Duration
}

func (r verifyResult) failed() bool { return r.ran && r.err != nil }

// verifyEvidence runs the ticket's `verify:` proof command in the repo root and
// renders a "## Verification" section recording the command, exit status, and
// (tail-capped) output. Like workEvidence it runs OUTSIDE the store lock.
func verifyEvidence(s *store.Store, id string) verifyResult {
	t, err := s.Get(id)
	if err != nil || strings.TrimSpace(t.Verify) == "" {
		return verifyResult{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), verifyTimeout)
	defer cancel()
	cmdStr := strings.TrimSpace(t.Verify)
	c := exec.CommandContext(ctx, "sh", "-c", cmdStr)
	c.Dir = filepath.Dir(s.Root())
	start := time.Now()
	out, err := c.CombinedOutput()
	res := verifyResult{
		command:  cmdStr,
		ran:      true,
		err:      err,
		output:   tailLines(string(out), verifyOutputTail),
		duration: time.Since(start),
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		res.exitCode = exitErr.ExitCode()
	}
	return res
}

// section renders the verifyResult as a "## Verification" markdown section;
// empty when nothing ran.
func (r verifyResult) section() string {
	if !r.ran {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Verification\n\n")
	status := "passed"
	if r.failed() {
		status = "FAILED"
	}
	note := ""
	if r.failed() && r.err != nil {
		if _, ok := r.err.(*exec.ExitError); !ok {
			note = fmt.Sprintf(" (%v)", r.err) // signal/timeout, not a plain exit code
		}
	}
	fmt.Fprintf(&b, "Ran by `pine close --evidence` on %s — **%s** (exit %d, %s)%s.\n\n",
		time.Now().UTC().Format("2006-01-02"), status, r.exitCode, r.duration.Round(time.Millisecond), note)
	fmt.Fprintf(&b, "Command:\n\n```\n%s\n```\n\n", r.command)
	if out := strings.TrimSpace(r.output); out == "" {
		b.WriteString("_(no output)_\n")
	} else {
		fmt.Fprintf(&b, "Output:\n\n```\n%s\n```\n", out)
	}
	return b.String()
}

// tailLines keeps the last max lines of s, prefixing a count of the trimmed ones.
func tailLines(s string, max int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= max {
		return s
	}
	return fmt.Sprintf("… (%d earlier lines trimmed)\n%s", len(lines)-max, strings.Join(lines[len(lines)-max:], "\n"))
}

// sectionBounds locates the "## <heading>" section: start is the index of its
// heading and end the index of the next "## " section (or the end of the body).
func sectionBounds(body, heading string) (start, end int, ok bool) {
	start = strings.Index(body, "## "+heading)
	if start < 0 {
		return 0, 0, false
	}
	end = len(body)
	if nl := strings.Index(body[start+1:], "\n## "); nl >= 0 {
		end = start + 1 + nl
	}
	return start, end, true
}

// appendSection replaces any existing "## <heading>" section in the body with
// content, or appends it at the end. Everything outside the section is
// preserved, so a ticket's original notes stay intact across re-runs and the
// Work Evidence and Verification sections can coexist in a stable order.
func appendSection(body, heading, content string) string {
	if idx, end, ok := sectionBounds(body, heading); ok {
		out := strings.TrimRight(body[:idx], "\n")
		if out != "" {
			out += "\n\n"
		}
		out += content
		if tail := strings.TrimSpace(body[end:]); tail != "" {
			out = strings.TrimRight(out, "\n") + "\n\n" + strings.TrimLeft(body[end:], "\n")
		}
		return out
	}
	if strings.TrimSpace(body) == "" {
		return content
	}
	return strings.TrimRight(body, "\n") + "\n\n" + content
}

// appendEvidence replaces any existing "## Work Evidence" section in the body
// with the new block, or appends it. Everything before the section is preserved
// so the ticket's original notes/description stay intact across re-runs.
func appendEvidence(body, evidence string) string {
	return appendSection(body, "Work Evidence", evidence)
}

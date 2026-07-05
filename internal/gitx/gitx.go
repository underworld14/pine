// Package gitx exposes read-only git awareness (branch, working-tree status,
// recent commits, tracked files) behind a small Client interface. The default
// implementation shells out to the git CLI, which is the fast path for real
// repos and is universally available for git-native workflows. When no git
// binary is present, a null client degrades gracefully (IsRepo == false).
package gitx

import (
	"bytes"
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Change is one entry in the working-tree status.
type Change struct {
	Path string `json:"path"`
	Code string `json:"code"` // porcelain code, e.g. "M", "A", "??"
}

// Commit is a recent commit.
type Commit struct {
	Hash    string    `json:"hash"`
	Subject string    `json:"subject"`
	Author  string    `json:"author"`
	When    time.Time `json:"when"`
}

// Status is a snapshot of git awareness.
type Status struct {
	IsRepo    bool     `json:"isRepo"`
	Branch    string   `json:"branch"`
	Dirty     bool     `json:"dirty"`
	Changes   []Change `json:"changes"`
	Truncated bool     `json:"truncated"`
	Commits   []Commit `json:"commits"`
}

// Branch is a local branch head with its tip commit pinned. The SHA is captured
// at enumeration time so callers can read that branch's tree via an immutable
// commit object — a branch deleted/renamed/moved afterwards cannot break a later
// ls-tree/show against the pinned SHA.
type Branch struct {
	Name       string    // %(refname:short), e.g. "feature/login"
	SHA        string    // %(objectname), the tip commit SHA — pin this for tree reads
	CommitDate time.Time // %(committerdate), tip commit time
}

// Client reads git state for a repository. Methods never error; problems yield
// an empty/not-a-repo Status (or an empty slice / ok=false for the tree readers).
type Client interface {
	IsRepo(ctx context.Context) bool
	Snapshot(ctx context.Context, commitLimit int) Status
	Files(ctx context.Context) []string

	// Branches lists local branch heads (refs/heads) with pinned tip SHAs.
	Branches(ctx context.Context) []Branch
	// ListTreeFiles lists file paths under pathspec in the committed tree at
	// rev (typically a pinned SHA), without checking anything out.
	ListTreeFiles(ctx context.Context, rev, pathspec string) []string
	// ShowFile returns the bytes of one file at rev. ok is false when the file
	// or rev does not exist.
	ShowFile(ctx context.Context, rev, path string) (content []byte, ok bool)
}

const maxChanges = 100

// New returns a git client anchored at repoRoot. If the git binary is missing,
// a null client is returned.
func New(repoRoot string) Client {
	if _, err := exec.LookPath("git"); err != nil {
		return nullClient{}
	}
	return &cli{repo: repoRoot}
}

type cli struct{ repo string }

func (c *cli) run(ctx context.Context, args ...string) (string, error) {
	out, err := c.runBytes(ctx, args...)
	return string(out), err
}

func (c *cli) runBytes(ctx context.Context, args ...string) ([]byte, error) {
	full := append([]string{"-C", c.repo}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	return out.Bytes(), err
}

func (c *cli) IsRepo(ctx context.Context) bool {
	out, err := c.run(ctx, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

func (c *cli) Snapshot(ctx context.Context, commitLimit int) Status {
	if !c.IsRepo(ctx) {
		return Status{Changes: []Change{}, Commits: []Commit{}}
	}
	s := Status{
		IsRepo:  true,
		Branch:  c.branch(ctx),
		Changes: []Change{},
		Commits: []Commit{},
	}
	s.Changes, s.Dirty, s.Truncated = c.changes(ctx)
	s.Commits = c.commits(ctx, commitLimit)
	return s
}

func (c *cli) branch(ctx context.Context) string {
	if out, err := c.run(ctx, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		if b := strings.TrimSpace(out); b != "" && b != "HEAD" {
			return b
		}
	}
	// Detached HEAD: label with the short SHA.
	if sha, err := c.run(ctx, "rev-parse", "--short", "HEAD"); err == nil && strings.TrimSpace(sha) != "" {
		return "detached @ " + strings.TrimSpace(sha)
	}
	// Fresh repo with no commits yet.
	if sym, err := c.run(ctx, "symbolic-ref", "--short", "HEAD"); err == nil {
		return strings.TrimSpace(sym) + " (no commits)"
	}
	return "(unknown)"
}

func (c *cli) changes(ctx context.Context) ([]Change, bool, bool) {
	out, err := c.run(ctx, "status", "--porcelain")
	if err != nil {
		return []Change{}, false, false
	}
	var changes []Change
	truncated := false
	for _, ln := range strings.Split(out, "\n") {
		if strings.TrimSpace(ln) == "" || len(ln) < 3 {
			continue
		}
		if len(changes) >= maxChanges {
			truncated = true
			break
		}
		code := strings.TrimSpace(ln[:2])
		path := strings.TrimSpace(ln[3:])
		changes = append(changes, Change{Path: path, Code: code})
	}
	if changes == nil {
		changes = []Change{}
	}
	return changes, len(changes) > 0, truncated
}

func (c *cli) commits(ctx context.Context, n int) []Commit {
	if n <= 0 {
		n = 10
	}
	out, err := c.run(ctx, "log", "-n", strconv.Itoa(n), "--format=%H%x1f%s%x1f%an%x1f%aI")
	if err != nil {
		return []Commit{}
	}
	var commits []Commit
	for _, ln := range strings.Split(out, "\n") {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		parts := strings.Split(ln, "\x1f")
		if len(parts) < 4 {
			continue
		}
		when, _ := time.Parse(time.RFC3339, parts[3])
		commits = append(commits, Commit{
			Hash:    shortHash(parts[0]),
			Subject: parts[1],
			Author:  parts[2],
			When:    when,
		})
	}
	if commits == nil {
		commits = []Commit{}
	}
	return commits
}

func (c *cli) Files(ctx context.Context) []string {
	out, err := c.run(ctx, "ls-files")
	if err != nil {
		return nil
	}
	var files []string
	for _, ln := range strings.Split(out, "\n") {
		if s := strings.TrimSpace(ln); s != "" {
			files = append(files, s)
		}
	}
	return files
}

// Branches lists local branch heads with pinned tip SHAs and commit dates.
// One for-each-ref call over refs/heads; \x1f separates fields on each line.
func (c *cli) Branches(ctx context.Context) []Branch {
	out, err := c.run(ctx,
		"for-each-ref",
		"--format=%(refname:short)%1f%(objectname)%1f%(committerdate:iso8601-strict)",
		"refs/heads",
	)
	if err != nil {
		return nil
	}
	var branches []Branch
	for _, ln := range strings.Split(out, "\n") {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		parts := strings.Split(ln, "\x1f")
		if len(parts) < 3 || parts[0] == "" || parts[1] == "" {
			continue
		}
		when, _ := time.Parse(time.RFC3339, parts[2])
		branches = append(branches, Branch{Name: parts[0], SHA: parts[1], CommitDate: when})
	}
	return branches
}

// ListTreeFiles lists file paths under pathspec in the committed tree at rev,
// without checking anything out. Paths are repo-root-relative, NUL-delimited.
func (c *cli) ListTreeFiles(ctx context.Context, rev, pathspec string) []string {
	out, err := c.run(ctx, "ls-tree", "-r", "--name-only", "-z", rev, "--", pathspec)
	if err != nil {
		return nil
	}
	var files []string
	for _, f := range strings.Split(out, "\x00") {
		if f != "" {
			files = append(files, f)
		}
	}
	return files
}

// ShowFile returns the bytes of one file at rev via `git show <rev>:<path>`.
func (c *cli) ShowFile(ctx context.Context, rev, path string) ([]byte, bool) {
	out, err := c.runBytes(ctx, "show", rev+":"+path)
	if err != nil {
		return nil, false
	}
	return out, true
}

func shortHash(h string) string {
	if len(h) > 8 {
		return h[:8]
	}
	return h
}

// nullClient is used when no git binary is available.
type nullClient struct{}

func (nullClient) IsRepo(context.Context) bool { return false }
func (nullClient) Snapshot(context.Context, int) Status {
	return Status{Changes: []Change{}, Commits: []Commit{}}
}
func (nullClient) Files(context.Context) []string                          { return nil }
func (nullClient) Branches(context.Context) []Branch                       { return nil }
func (nullClient) ListTreeFiles(context.Context, string, string) []string  { return nil }
func (nullClient) ShowFile(context.Context, string, string) ([]byte, bool) { return nil, false }

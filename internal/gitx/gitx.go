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
	"sort"
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

// RangeCommit is a commit together with its full message (subject and body),
// as returned by RangeLog.
type RangeCommit struct {
	Commit
	Message string `json:"message"`
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

	// Log returns commits that touch pathspec OR whose message contains grep
	// (either may be empty), de-duplicated by commit and newest first, capped
	// at limit.
	Log(ctx context.Context, pathspec, grep string, limit int) []Commit

	// CommitBefore returns the full SHA of the most recent commit at or before t,
	// or "" when there is no such commit, t is zero, or this is not a git repo.
	// Used to anchor "work evidence" to a ticket's creation time.
	CommitBefore(ctx context.Context, t time.Time) string

	// DiffStat returns a `git diff --stat`-style summary of all changes from base
	// to the working tree (committed-to-HEAD plus uncommitted). When base is empty,
	// only uncommitted workdir changes (vs HEAD) are reported. At most
	// diffStatMaxFiles files are listed (the summary line still counts them all).
	// Returns "" when there are no changes or git is unavailable.
	DiffStat(ctx context.Context, base string) string

	// ResolveCommit resolves rev (a SHA, tag, branch, HEAD~1, …) to a commit. ok
	// is false for an unknown rev, a non-commit object, an empty rev, or one that
	// starts with "-" (which git would read as an option).
	ResolveCommit(ctx context.Context, rev string) (sha string, c Commit, ok bool)

	// ChangedFiles lists the files under pathspec that differ between rev and the
	// working tree: committed since rev, staged, unstaged, and untracked files
	// that are not ignored. Paths are relative to the client's directory and the
	// result is sorted by path. Code is "A" (added or untracked), "M", or "D".
	// Returns nil when rev does not resolve or this is not a git repo.
	ChangedFiles(ctx context.Context, rev, pathspec string) []Change

	// RangeLog returns the commits in from..to, newest first, with full messages.
	// When excludePathspec is set, commits that only touch files under it are
	// dropped (git history simplification also drops merge commits then).
	RangeLog(ctx context.Context, from, to, excludePathspec string) []RangeCommit

	// CommitStat returns `git show --stat` output (one "<sha> <subject>" header
	// plus a capped file list per commit) for the given commits. Returns "" when
	// revs is empty, contains an option-like value, or this is not a git repo.
	CommitStat(ctx context.Context, revs []string) string
}

const maxChanges = 100

// diffStatMaxFiles caps how many files DiffStat and CommitStat list. Work
// evidence is written into the ticket file, so an uncapped stat from a ticket's
// creation to the working tree grows with every unrelated change in the repo.
const diffStatMaxFiles = 50

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

// Log unions "commits touching pathspec" with "commits whose message matches
// grep" (a literal, case-insensitive match), deduped by commit and newest
// first. Both queries are bounded, and the result is capped at limit.
func (c *cli) Log(ctx context.Context, pathspec, grep string, limit int) []Commit {
	if limit <= 0 {
		limit = 30
	}
	seen := map[string]bool{}
	var out []Commit
	base := []string{"log", "-n", strconv.Itoa(limit), "--format=%H%x1f%s%x1f%an%x1f%aI"}
	collect := func(args ...string) {
		res, err := c.run(ctx, append(append([]string{}, base...), args...)...)
		if err != nil {
			return
		}
		for _, ln := range strings.Split(res, "\n") {
			if strings.TrimSpace(ln) == "" {
				continue
			}
			parts := strings.Split(ln, "\x1f")
			if len(parts) < 4 || seen[parts[0]] {
				continue
			}
			seen[parts[0]] = true
			when, _ := time.Parse(time.RFC3339, parts[3])
			out = append(out, Commit{Hash: shortHash(parts[0]), Subject: parts[1], Author: parts[2], When: when})
		}
	}
	if grep != "" {
		collect("--grep="+grep, "--regexp-ignore-case", "--fixed-strings")
	}
	if pathspec != "" {
		collect("--", pathspec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].When.After(out[j].When) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func shortHash(h string) string {
	if len(h) > 8 {
		return h[:8]
	}
	return h
}

// CommitBefore returns the full SHA of the most recent commit at or before t.
// Empty when t is zero, there is no such commit, or this is not a git repo.
func (c *cli) CommitBefore(ctx context.Context, t time.Time) string {
	if t.IsZero() || !c.IsRepo(ctx) {
		return ""
	}
	// %cI is the committer date in strict ISO-8601; --before needs a date that
	// git can parse. RFC3339 is accepted.
	out, err := c.run(ctx, "log", "--before="+t.UTC().Format(time.RFC3339), "-1", "--format=%H")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// DiffStat returns a `git diff --stat` summary of changes from base to the
// working tree. When base is empty, only uncommitted workdir changes (vs HEAD)
// are reported. Empty when there are no changes or git is unavailable.
func (c *cli) DiffStat(ctx context.Context, base string) string {
	if !c.IsRepo(ctx) {
		return ""
	}
	args := []string{"diff", "--stat", "--stat-count=" + strconv.Itoa(diffStatMaxFiles)}
	if strings.TrimSpace(base) != "" {
		args = append(args, base)
	} else {
		// No base: compare the working tree to HEAD (uncommitted only).
		args = append(args, "HEAD")
	}
	out, err := c.run(ctx, args...)
	if err != nil {
		return ""
	}
	return strings.TrimRight(out, "\n")
}

// optionLike reports whether rev could be read by git as an option (or is
// otherwise unusable as a single revision argument).
func optionLike(rev string) bool {
	return rev == "" || strings.HasPrefix(rev, "-") || strings.ContainsAny(rev, "\n\x00")
}

// ResolveCommit resolves rev to its commit via `git show -s rev^{commit}`.
func (c *cli) ResolveCommit(ctx context.Context, rev string) (string, Commit, bool) {
	if optionLike(rev) {
		return "", Commit{}, false
	}
	out, err := c.run(ctx, "show", "-s", "--format=%H%x1f%s%x1f%an%x1f%aI", rev+"^{commit}", "--")
	if err != nil {
		return "", Commit{}, false
	}
	parts := strings.Split(strings.TrimRight(out, "\n"), "\x1f")
	if len(parts) < 4 || parts[0] == "" {
		return "", Commit{}, false
	}
	when, _ := time.Parse(time.RFC3339, parts[3])
	return parts[0], Commit{Hash: shortHash(parts[0]), Subject: parts[1], Author: parts[2], When: when}, true
}

// ChangedFiles unions `git diff --name-status <rev>` (tracked files: committed
// since rev, staged, unstaged) with `git ls-files --others --exclude-standard`
// (untracked, not ignored). --relative keeps paths relative to the repo dir when
// it is a subdirectory of the git top level.
func (c *cli) ChangedFiles(ctx context.Context, rev, pathspec string) []Change {
	sha, _, ok := c.ResolveCommit(ctx, rev)
	if !ok {
		return nil
	}
	codes := map[string]string{}
	diff, err := c.run(ctx, "diff", "--name-status", "--no-renames", "--relative", "-z", sha, "--", pathspec)
	if err != nil {
		return nil
	}
	toks := strings.Split(diff, "\x00")
	for i := 0; i+1 < len(toks); i += 2 {
		code := "M" // type changes (T) and anything unusual read as modified
		if toks[i] == "A" || toks[i] == "D" {
			code = toks[i]
		}
		codes[toks[i+1]] = code
	}
	others, err := c.run(ctx, "ls-files", "--others", "--exclude-standard", "-z", "--", pathspec)
	if err == nil {
		for _, p := range strings.Split(others, "\x00") {
			if p != "" {
				codes[p] = "A"
			}
		}
	}
	out := make([]Change, 0, len(codes))
	for p, code := range codes {
		out = append(out, Change{Path: p, Code: code})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// RangeLog lists from..to with full messages; \x1e starts each record and \x1f
// separates fields (the message body is last, so it may hold anything else).
func (c *cli) RangeLog(ctx context.Context, from, to, excludePathspec string) []RangeCommit {
	if optionLike(from) || optionLike(to) {
		return nil
	}
	args := []string{"log", "--format=%x1e%H%x1f%s%x1f%an%x1f%aI%x1f%B", from + ".." + to}
	if excludePathspec != "" {
		// ":/" is the whole repository, so the exclusion has a positive pathspec
		// to subtract from; the exclusion itself is relative to the client dir.
		args = append(args, "--", ":/", ":(exclude)"+excludePathspec)
	}
	out, err := c.run(ctx, args...)
	if err != nil {
		return nil
	}
	var commits []RangeCommit
	for _, rec := range strings.Split(out, "\x1e") {
		parts := strings.SplitN(rec, "\x1f", 5)
		if len(parts) < 5 || strings.TrimSpace(parts[0]) == "" {
			continue
		}
		when, _ := time.Parse(time.RFC3339, parts[3])
		commits = append(commits, RangeCommit{
			Commit:  Commit{Hash: shortHash(strings.TrimSpace(parts[0])), Subject: parts[1], Author: parts[2], When: when},
			Message: strings.TrimSpace(parts[4]),
		})
	}
	return commits
}

// CommitStat shows a capped --stat for each commit. -m --first-parent makes a
// merge commit report its change against the first parent instead of nothing.
func (c *cli) CommitStat(ctx context.Context, revs []string) string {
	if len(revs) == 0 || !c.IsRepo(ctx) {
		return ""
	}
	args := []string{"show", "--stat", "--stat-count=" + strconv.Itoa(diffStatMaxFiles), "--format=%h %s", "-m", "--first-parent"}
	for _, r := range revs {
		if optionLike(r) {
			return ""
		}
		args = append(args, r)
	}
	out, err := c.run(ctx, append(args, "--")...)
	if err != nil {
		return ""
	}
	return strings.TrimRight(out, "\n")
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
func (nullClient) Log(context.Context, string, string, int) []Commit       { return nil }
func (nullClient) CommitBefore(context.Context, time.Time) string          { return "" }
func (nullClient) DiffStat(context.Context, string) string                 { return "" }
func (nullClient) ResolveCommit(context.Context, string) (string, Commit, bool) {
	return "", Commit{}, false
}
func (nullClient) ChangedFiles(context.Context, string, string) []Change { return nil }
func (nullClient) RangeLog(context.Context, string, string, string) []RangeCommit {
	return nil
}
func (nullClient) CommitStat(context.Context, []string) string { return "" }

// Package doctor validates a .pine workspace: config/board schemas, ticket
// integrity, dependency/epic consistency, and attachment health. It is read-only
// and reports every problem it finds so `pine doctor` can surface them at once.
package doctor

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/underworld14/pine/internal/config"
	"github.com/underworld14/pine/internal/learning"
	"github.com/underworld14/pine/internal/store"
	"github.com/underworld14/pine/internal/syncignore"
	"github.com/underworld14/pine/internal/ticket"
)

// Level is a finding severity.
type Level int

const (
	LevelOK Level = iota
	LevelWarn
	LevelError
)

// Finding is one doctor result. Code is a stable machine-readable slug for
// fixable findings (empty for informational ones). Fix, when non-nil, performs
// a mechanical repair; findings that need human judgment leave it nil.
type Finding struct {
	Level Level
	Code  string
	Msg   string
	Fix   func(s *store.Store) error `json:"-"`
}

// Fixable reports whether this finding can be auto-repaired.
func (f Finding) Fixable() bool { return f.Fix != nil }

// Report collects findings.
type Report struct {
	Findings []Finding
}

func (r *Report) ok(msg string) { r.Findings = append(r.Findings, Finding{Level: LevelOK, Msg: msg}) }
func (r *Report) warn(msg string) {
	r.Findings = append(r.Findings, Finding{Level: LevelWarn, Msg: msg})
}
func (r *Report) err(msg string) {
	r.Findings = append(r.Findings, Finding{Level: LevelError, Msg: msg})
}

// warnFix records a fixable warning with a stable code and a repair closure.
func (r *Report) warnFix(code, msg string, fix func(s *store.Store) error) {
	r.Findings = append(r.Findings, Finding{Level: LevelWarn, Code: code, Msg: msg, Fix: fix})
}

// HasErrors reports whether any error-level finding exists.
func (r *Report) HasErrors() bool {
	for _, f := range r.Findings {
		if f.Level == LevelError {
			return true
		}
	}
	return false
}

// FixableCount returns how many findings can be auto-repaired.
func (r *Report) FixableCount() int {
	n := 0
	for _, f := range r.Findings {
		if f.Fixable() {
			n++
		}
	}
	return n
}

var ticketFileRe = regexp.MustCompile(`^[A-Z][A-Z0-9]*-[0-9a-hj-km-np-tv-z]+\.md$`)

// Run performs all checks against the store.
func Run(s *store.Store) *Report {
	r := &Report{}
	cfg := s.Config()
	board := s.Board()
	root := s.Root()

	for _, p := range cfg.Validate() {
		r.err("config.json: " + p)
	}
	for _, p := range board.Validate() {
		r.err("board.json: " + p)
	}
	if !r.HasErrors() {
		r.ok("config.json and board.json are valid")
	}

	if cfg.CrossBranch.Enabled && cfg.IDStyle != "hash" {
		r.warn("crossBranch is enabled but idStyle is \"" + cfg.IDStyle +
			"\" — cross-branch aggregation is disabled because sequential IDs collide across branches")
	}

	all := s.All()
	byID := map[string]*ticket.Ticket{}
	for _, t := range all {
		byID[t.ID] = t
	}

	for _, t := range all {
		if t.Degraded {
			r.err(t.ID + ": malformed (" + strings.Join(t.Warnings, "; ") + ")")
			continue
		}
		for _, w := range t.Warnings {
			r.warn(t.ID + ": " + w)
		}
		id := t.ID
		if t.FrontmatterID != "" && t.FrontmatterID != t.ID {
			r.warnFix("frontmatter-id-mismatch", t.ID+": frontmatter id is "+t.FrontmatterID+" (does not match filename)",
				func(s *store.Store) error {
					// A no-op update re-serializes with the canonical, filename-derived id.
					_, err := s.Update(id, func(*ticket.Ticket) error { return nil })
					return err
				})
		}
		if t.Status != "" && !board.HasStatus(t.Status) {
			r.warn(t.ID + ": status " + t.Status + " matches no board column")
		}
		if t.Priority != "" && !cfg.HasPriority(t.Priority) {
			r.warn(t.ID + ": priority " + t.Priority + " is not configured")
		}
		for _, dep := range t.Deps {
			if byID[dep] == nil {
				dep := dep
				r.warnFix("dangling-dep", t.ID+": dangling dependency "+dep, func(s *store.Store) error {
					_, err := s.Update(id, func(tt *ticket.Ticket) error {
						tt.Deps = removeString(tt.Deps, dep)
						return nil
					})
					return err
				})
			}
		}
		if t.Parent != "" {
			clearParent := func(s *store.Store) error {
				_, err := s.Update(id, func(tt *ticket.Ticket) error { tt.Parent = ""; return nil })
				return err
			}
			if p := byID[t.Parent]; p == nil {
				r.warnFix("dangling-parent", t.ID+": parent "+t.Parent+" does not exist", clearParent)
			} else if p.Prefix() != "EPIC" {
				r.warnFix("parent-not-epic", t.ID+": parent "+t.Parent+" is not an epic", clearParent)
			}
		}
		checkAttachmentRefs(r, s, t)
	}

	for _, cyc := range s.Graph().Cycles() {
		r.err("dependency cycle among: " + strings.Join(cyc, ", "))
	}

	checkStrays(r, root, "tickets", "ticket")
	checkLearnings(r, s, byID)
	checkAttachmentDirs(r, s, byID, cfg.Attachments.MaxVideoMB)
	checkGitignore(r, root)
	checkMergeDriver(r, root)

	if !r.HasErrors() {
		r.ok("no problems found")
	}
	return r
}

func checkAttachmentRefs(r *Report, s *store.Store, t *ticket.Ticket) {
	refs := ticket.AttachmentRefs(t.Body)
	if len(refs) == 0 {
		return
	}
	actual := map[string]bool{}
	for _, a := range s.Attachments(t.ID) {
		actual[a.Name] = true
	}
	for _, ref := range refs {
		if !strings.Contains(ref, "attachments/") {
			continue
		}
		name := filepath.Base(ref)
		if !actual[name] {
			r.err(t.ID + ": references missing attachment " + ref)
		}
	}
}

// checkStrays scans root/dir for entries that don't look like a valid
// <label> filename (e.g. tickets/ or learnings/, both use the same ID
// alphabet). When a stray file carries a valid frontmatter id whose canonical
// filename is free, it can be auto-renamed; otherwise the fix needs judgment.
func checkStrays(r *Report, root, dir, label string) {
	dirPath := filepath.Join(root, dir)
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return
	}
	present := map[string]bool{}
	for _, e := range entries {
		present[e.Name()] = true
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}
		if ticketFileRe.MatchString(name) {
			continue
		}
		msg := dir + "/" + name + ": stray file (not a valid " + label + " name)"
		if target := strayRenameTarget(dirPath, name, present); target != "" {
			from := filepath.Join(dirPath, name)
			to := filepath.Join(dirPath, target)
			r.warnFix("stray-file", msg+" — can be renamed to "+target, func(*store.Store) error {
				return os.Rename(from, to)
			})
		} else {
			r.warn(msg)
		}
	}
}

// strayRenameTarget returns the canonical "<id>.md" filename a stray file could
// be renamed to — derived from its frontmatter id — or "" when it has no valid
// id or that name is already taken. The frontmatter id key is identical for
// tickets and learnings, so ticket.Parse extracts it for both.
func strayRenameTarget(dirPath, name string, present map[string]bool) string {
	data, err := os.ReadFile(filepath.Join(dirPath, name))
	if err != nil {
		return ""
	}
	t := ticket.Parse(strings.TrimSuffix(name, ".md"), data)
	fid := strings.TrimSpace(t.FrontmatterID)
	if fid == "" || !ticket.ValidID(fid) {
		return ""
	}
	target := fid + ".md"
	if present[target] {
		return ""
	}
	return target
}

// removeString returns vals with the first occurrence of v removed.
func removeString(vals []string, v string) []string {
	out := make([]string, 0, len(vals))
	for _, x := range vals {
		if x == v {
			continue
		}
		out = append(out, x)
	}
	return out
}

func checkLearnings(r *Report, s *store.Store, byID map[string]*ticket.Ticket) {
	allLearnings := s.AllLearnings()
	learningByID := map[string]*learning.Learning{}
	for _, l := range allLearnings {
		learningByID[l.ID] = l
	}
	for _, l := range allLearnings {
		if l.Degraded {
			r.err(l.ID + ": malformed (" + strings.Join(l.Warnings, "; ") + ")")
			continue
		}
		for _, w := range l.Warnings {
			r.warn(l.ID + ": " + w)
		}
		lid := l.ID
		if l.FrontmatterID != "" && l.FrontmatterID != l.ID {
			r.warnFix("learning-frontmatter-id-mismatch", l.ID+": frontmatter id is "+l.FrontmatterID+" (does not match filename)",
				func(s *store.Store) error {
					_, err := s.UpdateLearning(lid, func(*learning.Learning) error { return nil })
					return err
				})
		}
		if l.Scope != "" && !learning.ValidScope(l.Scope) {
			r.warn(l.ID + ": scope " + l.Scope + " is not valid (expected global, ticket, or component)")
		}
		if l.SourceAgent != "" && !learning.ValidSourceAgent(l.SourceAgent) {
			r.warn(l.ID + ": source_agent " + l.SourceAgent + " is not recognized")
		}
		if l.Scope == learning.ScopeTicket {
			if l.Ticket == "" {
				r.warn(l.ID + ": scope is ticket but ticket field is empty")
			} else if byID[l.Ticket] == nil {
				// Dropping the ticket ref would leave an invalid ticket-scoped
				// learning, so this needs human judgment (re-point or delete).
				r.warn(l.ID + ": dangling ticket ref " + l.Ticket)
			}
		}
		if l.Scope == learning.ScopeComponent && l.Component == "" {
			r.warn(l.ID + ": scope is component but component field is empty")
		}
		if l.Supersedes != "" {
			if _, ok := learningByID[l.Supersedes]; !ok {
				r.warnFix("dangling-supersedes", l.ID+": dangling supersedes ref "+l.Supersedes,
					func(s *store.Store) error {
						_, err := s.UpdateLearning(lid, func(m *learning.Learning) error {
							m.Supersedes = ""
							return nil
						})
						return err
					})
			}
		}
		for _, cite := range l.Cites {
			cite = strings.TrimSpace(cite)
			if cite == "" {
				continue
			}
			if !s.CiteExists(cite) {
				cite := cite
				r.warnFix("dangling-cite", l.ID+": dangling cite "+cite, func(s *store.Store) error {
					_, err := s.UpdateLearning(lid, func(m *learning.Learning) error {
						m.Cites = removeString(m.Cites, cite)
						return nil
					})
					return err
				})
			}
		}
	}
	fwd, _ := learning.BuildEdges(allLearnings)
	for _, cyc := range learning.FindCycles(fwd) {
		r.err("supersede cycle among: " + strings.Join(cyc, ", "))
	}
	checkStrays(r, s.Root(), "learnings", "learning")
}

func checkAttachmentDirs(r *Report, s *store.Store, byID map[string]*ticket.Ticket, maxVideoMB int) {
	for _, id := range s.AttachmentDirs() {
		if byID[id] == nil {
			r.warn("attachments/" + id + ": orphaned directory (no such ticket)")
			continue
		}
		for _, a := range s.Attachments(id) {
			if a.Kind == "video" && maxVideoMB > 0 && a.Size > int64(maxVideoMB)*1024*1024 {
				r.warn(id + ": video " + a.Name + " exceeds " + strconv.Itoa(maxVideoMB) + "MB (bloats the repo)")
			}
		}
	}
}

// checkMergeDriver warns when .gitattributes maps ticket files to the pine
// merge driver but this clone hasn't configured it (a fresh clone that skipped
// 'pine setup merge' — merges would fall back to raw text conflicts).
func checkMergeDriver(r *Report, pineRoot string) {
	repoRoot := filepath.Dir(pineRoot)
	data, err := os.ReadFile(filepath.Join(repoRoot, ".gitattributes"))
	if err != nil || !strings.Contains(string(data), "merge=pine") {
		return
	}
	out, err := exec.Command("git", "-C", repoRoot, "config", "--get", "merge.pine.driver").Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		r.warn(".gitattributes references merge=pine but git config merge.pine.driver is unset — run 'pine setup merge'")
	}
}

func checkGitignore(r *Report, pineRoot string) {
	repoRoot := filepath.Dir(pineRoot)
	f, err := os.Open(filepath.Join(repoRoot, ".gitignore"))
	if err == nil {
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			trimmed := strings.TrimSuffix(strings.TrimPrefix(line, "/"), "/")
			if trimmed == ".pine" {
				r.warn(".pine is gitignored — Pine data is meant to be committed")
				return
			}
		}
	}

	// Nested .pine/.gitignore may intentionally ignore attachments/ (default)
	// or tickets/ when sync.tickets is false. Only warn on unexpected ticket ignore.
	nested, err := os.ReadFile(filepath.Join(pineRoot, ".gitignore"))
	if err != nil {
		return
	}
	body := string(nested)
	ticketsIgnored := nestedIgnoresTickets(body)
	if !ticketsIgnored {
		return
	}
	cfg, err := config.Load(filepath.Join(pineRoot, "config.json"))
	if err == nil && !cfg.Sync.Tickets {
		return // intentional local tickets
	}
	r.warn(".pine/tickets/ is gitignored but config expects tracked tickets — run 'pine setup sync' or fix .pine/.gitignore")
}

func nestedIgnoresTickets(body string) bool {
	prefs := syncignore.ParseManagedBlock(body)
	if !prefs.Tickets {
		return true
	}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		trimmed := strings.TrimSuffix(strings.TrimPrefix(line, "/"), "/")
		if trimmed == "tickets" {
			return true
		}
	}
	return false
}

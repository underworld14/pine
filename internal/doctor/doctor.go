// Package doctor validates a .pine workspace: config/board schemas, ticket
// integrity, dependency/epic consistency, and attachment health. It is read-only
// and reports every problem it finds so `pine doctor` can surface them at once.
package doctor

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/underworld14/pine/internal/learning"
	"github.com/underworld14/pine/internal/store"
	"github.com/underworld14/pine/internal/ticket"
)

// Level is a finding severity.
type Level int

const (
	LevelOK Level = iota
	LevelWarn
	LevelError
)

// Finding is one doctor result.
type Finding struct {
	Level Level
	Msg   string
}

// Report collects findings.
type Report struct {
	Findings []Finding
}

func (r *Report) ok(msg string)   { r.Findings = append(r.Findings, Finding{LevelOK, msg}) }
func (r *Report) warn(msg string) { r.Findings = append(r.Findings, Finding{LevelWarn, msg}) }
func (r *Report) err(msg string)  { r.Findings = append(r.Findings, Finding{LevelError, msg}) }

// HasErrors reports whether any error-level finding exists.
func (r *Report) HasErrors() bool {
	for _, f := range r.Findings {
		if f.Level == LevelError {
			return true
		}
	}
	return false
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
		if t.FrontmatterID != "" && t.FrontmatterID != t.ID {
			r.warn(t.ID + ": frontmatter id is " + t.FrontmatterID + " (does not match filename)")
		}
		if t.Status != "" && !board.HasStatus(t.Status) {
			r.warn(t.ID + ": status " + t.Status + " matches no board column")
		}
		if t.Priority != "" && !cfg.HasPriority(t.Priority) {
			r.warn(t.ID + ": priority " + t.Priority + " is not configured")
		}
		for _, dep := range t.Deps {
			if byID[dep] == nil {
				r.warn(t.ID + ": dangling dependency " + dep)
			}
		}
		if t.Parent != "" {
			if p := byID[t.Parent]; p == nil {
				r.warn(t.ID + ": parent " + t.Parent + " does not exist")
			} else if p.Prefix() != "EPIC" {
				r.warn(t.ID + ": parent " + t.Parent + " is not an epic")
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
// alphabet).
func checkStrays(r *Report, root, dir, label string) {
	entries, err := os.ReadDir(filepath.Join(root, dir))
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}
		if !ticketFileRe.MatchString(name) {
			r.warn(dir + "/" + name + ": stray file (not a valid " + label + " name)")
		}
	}
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
		if l.FrontmatterID != "" && l.FrontmatterID != l.ID {
			r.warn(l.ID + ": frontmatter id is " + l.FrontmatterID + " (does not match filename)")
		}
		if l.Scope != "" && !learning.ValidScope(l.Scope) {
			r.warn(l.ID + ": scope " + l.Scope + " is not valid (expected global or ticket)")
		}
		if l.SourceAgent != "" && !learning.ValidSourceAgent(l.SourceAgent) {
			r.warn(l.ID + ": source_agent " + l.SourceAgent + " is not recognized")
		}
		if l.Scope == learning.ScopeTicket {
			if l.Ticket == "" {
				r.warn(l.ID + ": scope is ticket but ticket field is empty")
			} else if byID[l.Ticket] == nil {
				r.warn(l.ID + ": dangling ticket ref " + l.Ticket)
			}
		}
		if l.Supersedes != "" {
			if _, ok := learningByID[l.Supersedes]; !ok {
				r.warn(l.ID + ": dangling supersedes ref " + l.Supersedes)
			}
		}
		for _, cite := range l.Cites {
			cite = strings.TrimSpace(cite)
			if cite == "" {
				continue
			}
			if !s.CiteExists(cite) {
				r.warn(l.ID + ": dangling cite " + cite)
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

func checkGitignore(r *Report, pineRoot string) {
	repoRoot := filepath.Dir(pineRoot)
	f, err := os.Open(filepath.Join(repoRoot, ".gitignore"))
	if err != nil {
		return
	}
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

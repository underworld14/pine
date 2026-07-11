package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/underworld14/pine/internal/learning"
)

const learningPrefix = learning.Prefix

// CreateLearningReq describes a new learning.
type CreateLearningReq struct {
	Text        string
	Scope       string // default "global"
	Tags        []string
	Ticket      string
	SourceAgent string   // default "manual"
	Supersedes  string   // optional learning ID this replaces
	Cites       []string // optional repo-relative paths
}

// CreateLearning allocates an ID, writes the file atomically, and returns the learning.
func (s *Store) CreateLearning(req CreateLearningReq) (*learning.Learning, error) {
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return nil, errors.New("learning text is required")
	}
	scope := strings.ToLower(strings.TrimSpace(req.Scope))
	if scope == "" {
		scope = learning.ScopeGlobal
	}
	if !learning.ValidScope(scope) {
		return nil, errors.New("invalid scope: must be global or ticket")
	}
	source := strings.ToLower(strings.TrimSpace(req.SourceAgent))
	if source == "" {
		source = learning.SourceManual
	}
	if !learning.ValidSourceAgent(source) {
		return nil, errors.New("invalid source_agent")
	}
	ticketID := strings.TrimSpace(req.Ticket)
	if scope == learning.ScopeTicket {
		if ticketID == "" {
			return nil, errors.New("--ticket is required when scope is ticket")
		}
	} else {
		// A global learning carries no ticket reference, regardless of what
		// the caller passed — scope is the single source of truth.
		ticketID = ""
	}
	supersedes := strings.TrimSpace(req.Supersedes)
	cites := normalizeCites(req.Cites)
	if err := validateCites(cites); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if scope == learning.ScopeTicket {
		if _, ok := s.cache[ticketID]; !ok {
			return nil, errors.New("ticket " + ticketID + " does not exist")
		}
	}
	if supersedes != "" {
		if _, ok := s.learningCache[supersedes]; !ok {
			return nil, errors.New("supersedes target " + supersedes + " does not exist")
		}
	}

	id, err := s.allocIDIn(s.learningsDir(), s.learningPath, learningPrefix, "learning")
	if err != nil {
		return nil, err
	}
	if supersedes != "" {
		all := make([]*learning.Learning, 0, len(s.learningCache))
		for _, l := range s.learningCache {
			all = append(all, l)
		}
		fwd, _ := learning.BuildEdges(all)
		if cyc := learning.WouldCycle(fwd, id, supersedes); cyc != nil {
			_ = os.Remove(s.learningPath(id))
			return nil, errors.New("that supersede would create a cycle among: " + strings.Join(cyc, ", "))
		}
	}
	body := text
	if !strings.HasPrefix(body, "\n") {
		body = "\n" + body
	}
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	now := s.now().UTC()
	l := &learning.Learning{
		ID:          id,
		Scope:       scope,
		Tags:        normalizeTags(req.Tags),
		Ticket:      ticketID,
		SourceAgent: source,
		Supersedes:  supersedes,
		Cites:       cites,
		Created:     now,
		Body:        body,
	}
	if err := s.saveLearning(l); err != nil {
		_ = os.Remove(s.learningPath(id))
		return nil, err
	}
	return cloneLearning(l), nil
}

func (s *Store) learningsDir() string { return filepath.Join(s.root, dirLearnings) }
func (s *Store) learningPath(id string) string {
	return filepath.Join(s.learningsDir(), id+".md")
}

func (s *Store) saveLearning(l *learning.Learning) error {
	l.Scope = strings.ToLower(strings.TrimSpace(l.Scope))
	l.SourceAgent = strings.ToLower(strings.TrimSpace(l.SourceAgent))
	data := l.Serialize()
	if err := atomicWrite(s.learningPath(l.ID), data); err != nil {
		return err
	}
	s.learningCache[l.ID] = cloneLearning(l)
	s.learningHash[l.ID] = hashBytes(data)
	return nil
}

// scanLearnings reads and parses every learning file into the cache. Learning
// filenames share the ticket ID alphabet, so the same ticketFileRe applies.
func (s *Store) scanLearnings() error {
	entries, err := os.ReadDir(s.learningsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !ticketFileRe.MatchString(e.Name()) {
			continue
		}
		_ = s.loadLearningFile(e.Name())
	}
	return nil
}

func (s *Store) loadLearningFile(filename string) error {
	id := strings.TrimSuffix(filename, ".md")
	path := filepath.Join(s.learningsDir(), filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	l := learning.Parse(id, data)
	applyLearningMtimeFallback(l, path)
	s.learningCache[id] = l
	s.learningHash[id] = hashBytes(data)
	return nil
}

// ReloadLearning re-reads one learning file after an external change. Dedupes
// by content hash so a write echoed by the filesystem is a no-op.
func (s *Store) ReloadLearning(path string) (Change, error) {
	filename := filepath.Base(path)
	if !ticketFileRe.MatchString(filename) {
		return Change{}, nil
	}
	id := strings.TrimSuffix(filename, ".md")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			s.mu.Lock()
			_, existed := s.learningCache[id]
			delete(s.learningCache, id)
			delete(s.learningHash, id)
			s.mu.Unlock()
			return Change{ID: id, Removed: existed, Changed: existed}, nil
		}
		return Change{ID: id}, err
	}

	h := hashBytes(data)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.learningHash[id] == h {
		return Change{ID: id}, nil
	}
	l := learning.Parse(id, data)
	applyLearningMtimeFallback(l, path)
	s.learningCache[id] = l
	s.learningHash[id] = h
	return Change{ID: id, Changed: true}, nil
}

func applyLearningMtimeFallback(l *learning.Learning, path string) {
	if !l.Created.IsZero() {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	l.Created = info.ModTime().UTC()
}

// CiteExists reports whether a repo-relative path cited by a learning exists
// on disk as a regular file. A path that resolves to a directory is never
// considered to exist — citations are meant to point at specific files.
func (s *Store) CiteExists(rel string) bool {
	info, err := os.Stat(filepath.Join(filepath.Dir(s.root), filepath.FromSlash(rel)))
	return err == nil && !info.IsDir()
}

// CitationStaleIDs reports, for each of the given learnings, whether any of
// its cited paths is missing. Existence is memoized per path for the
// duration of the call so a path cited by multiple learnings is stat'd once.
func (s *Store) CitationStaleIDs(learnings []*learning.Learning) map[string]bool {
	cache := map[string]bool{}
	exists := func(rel string) bool {
		if v, ok := cache[rel]; ok {
			return v
		}
		v := s.CiteExists(rel)
		cache[rel] = v
		return v
	}
	out := make(map[string]bool, len(learnings))
	for _, l := range learnings {
		out[l.ID] = learning.IsCitationStale(learning.MissingCitedPaths(l.Cites, exists))
	}
	return out
}

// LearningFilter selects a subset of learnings. Empty fields match everything.
type LearningFilter struct {
	Scope             string
	Tags              []string // all tags must be present (AND)
	Ticket            string
	IncludeSuperseded bool // when false (default), hide entries that another learning supersedes
	IncludeStale      bool // when false (default), hide entries with a missing cited path
}

func (f LearningFilter) matches(l *learning.Learning) bool {
	if f.Scope != "" && strings.ToLower(strings.TrimSpace(f.Scope)) != l.Scope {
		return false
	}
	if f.Ticket != "" && l.Ticket != f.Ticket {
		return false
	}
	for _, want := range f.Tags {
		want = strings.ToLower(strings.TrimSpace(want))
		found := false
		for _, t := range l.Tags {
			if strings.ToLower(t) == want {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// ListLearnings returns learnings matching the filter, newest first.
// By default, superseded and citation-stale learnings are excluded unless
// IncludeSuperseded / IncludeStale are set.
func (s *Store) ListLearnings(f LearningFilter) []*learning.Learning {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var all []*learning.Learning
	for _, l := range s.learningCache {
		all = append(all, l)
	}
	var rev map[string][]string
	if !f.IncludeSuperseded {
		_, rev = learning.BuildEdges(all)
	}
	var stale map[string]bool
	if !f.IncludeStale {
		stale = s.CitationStaleIDs(all)
	}
	var out []*learning.Learning
	for _, l := range all {
		if !f.IncludeSuperseded && learning.IsSuperseded(rev, l.ID) {
			continue
		}
		if !f.IncludeStale && stale[l.ID] {
			continue
		}
		if f.matches(l) {
			out = append(out, cloneLearning(l))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Created.Equal(out[j].Created) {
			return out[i].ID > out[j].ID
		}
		return out[i].Created.After(out[j].Created)
	})
	return out
}

// AllLearnings returns every learning including superseded and citation-stale, newest first.
func (s *Store) AllLearnings() []*learning.Learning {
	return s.ListLearnings(LearningFilter{IncludeSuperseded: true, IncludeStale: true})
}

// GetLearning returns a copy of a learning by ID.
func (s *Store) GetLearning(id string) (*learning.Learning, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	l, ok := s.learningCache[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneLearning(l), nil
}

func cloneLearning(l *learning.Learning) *learning.Learning {
	c := *l
	c.Tags = append([]string(nil), l.Tags...)
	c.Cites = append([]string(nil), l.Cites...)
	c.Extra = append([]learning.ExtraField(nil), l.Extra...)
	c.Warnings = append([]string(nil), l.Warnings...)
	return &c
}

// validateCites rejects cite paths that could escape the repository root:
// absolute paths and any path containing a ".." segment.
func validateCites(paths []string) error {
	for _, p := range paths {
		if filepath.IsAbs(p) {
			return fmt.Errorf("cite path must be repo-relative, not absolute: %q", p)
		}
		for _, part := range strings.Split(p, "/") {
			if part == ".." {
				return fmt.Errorf("cite path must not contain '..': %q", p)
			}
		}
	}
	return nil
}

func normalizeStrings(vals []string, transform func(string) string) []string {
	if len(vals) == 0 {
		return nil
	}
	out := make([]string, 0, len(vals))
	seen := map[string]bool{}
	for _, v := range vals {
		v = transform(strings.TrimSpace(v))
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func normalizeCites(paths []string) []string {
	return normalizeStrings(paths, filepath.ToSlash)
}

func normalizeTags(tags []string) []string {
	return normalizeStrings(tags, strings.ToLower)
}

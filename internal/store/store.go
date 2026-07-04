// Package store is Pine's single write path over a .pine/ directory. Every
// mutation (from the CLI or HTTP API) goes through it and writes atomically;
// external writers (AI agents, editors) bypass it and the watcher reconciles.
// The in-memory cache mirrors disk and is always rebuildable from it.
package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/underworld14/pine/internal/config"
	"github.com/underworld14/pine/internal/ticket"
)

// Sentinel errors returned by the store.
var (
	ErrNotFound    = errors.New("ticket not found")
	ErrExists      = errors.New("ticket already exists")
	ErrDegraded    = errors.New("ticket is malformed and cannot be edited through Pine")
	ErrUnknownType = errors.New("unknown ticket type")
)

// ticketFileRe matches valid ticket filenames; anything else in tickets/ is
// ignored (editor droppings, .DS_Store, etc.).
var ticketFileRe = regexp.MustCompile(`^[A-Z][A-Z0-9]*-[0-9]+\.md$`)

// Subdirectory and file names inside .pine/.
const (
	dirTickets     = "tickets"
	dirAttachments = "attachments"
	dirTemplates   = "templates"
	dirPrompts     = "prompts"
	fileConfig     = "config.json"
	fileBoard      = "board.json"
)

// Store holds the parsed contents of a .pine/ directory.
type Store struct {
	root string // absolute path to .pine

	mu    sync.RWMutex
	cache map[string]*ticket.Ticket
	hash  map[string]string // ticket id -> sha256 of on-disk bytes
	cfg   *config.Config
	board *config.Board

	now func() time.Time // injectable clock (tests)
}

// Open loads config, board, and all tickets from pineDir (the path to .pine).
func Open(pineDir string) (*Store, error) {
	abs, err := filepath.Abs(pineDir)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(filepath.Join(abs, fileConfig))
	if err != nil {
		return nil, err
	}
	board, err := config.LoadBoard(filepath.Join(abs, fileBoard))
	if err != nil {
		return nil, err
	}
	s := &Store{
		root:  abs,
		cache: map[string]*ticket.Ticket{},
		hash:  map[string]string{},
		cfg:   cfg,
		board: board,
		now:   time.Now,
	}
	if err := s.scanTickets(); err != nil {
		return nil, err
	}
	return s, nil
}

// Root returns the absolute path to the .pine directory.
func (s *Store) Root() string { return s.root }

// SetClock overrides the time source; used by tests for deterministic output.
func (s *Store) SetClock(now func() time.Time) { s.now = now }

// Config returns the loaded configuration.
func (s *Store) Config() *config.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// Board returns the loaded board.
func (s *Store) Board() *config.Board {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.board
}

func (s *Store) ticketsDir() string { return filepath.Join(s.root, dirTickets) }
func (s *Store) ticketPath(id string) string {
	return filepath.Join(s.ticketsDir(), id+".md")
}

// scanTickets reads and parses every ticket file into the cache.
func (s *Store) scanTickets() error {
	entries, err := os.ReadDir(s.ticketsDir())
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
		if err := s.loadTicketFile(e.Name()); err != nil {
			// A single unreadable file must not abort the scan.
			continue
		}
	}
	return nil
}

// loadTicketFile parses one ticket file into the cache, applying mtime
// fallbacks for missing timestamps.
func (s *Store) loadTicketFile(filename string) error {
	id := strings.TrimSuffix(filename, ".md")
	path := filepath.Join(s.ticketsDir(), filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	t := ticket.Parse(id, data)
	applyMtimeFallback(t, path)
	s.cache[id] = t
	s.hash[id] = hashBytes(data)
	return nil
}

// applyMtimeFallback fills zero timestamps from the file's modification time so
// agent-written tickets that omit created/updated still sort sensibly.
func applyMtimeFallback(t *ticket.Ticket, path string) {
	if !t.Created.IsZero() && !t.Updated.IsZero() {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	mt := info.ModTime().UTC()
	if t.Created.IsZero() {
		t.Created = mt
	}
	if t.Updated.IsZero() {
		t.Updated = mt
	}
}

// Filter selects a subset of tickets. Empty fields match everything.
type Filter struct {
	Status string
	Type   string // ID prefix, e.g. "BUG"
	Label  string
	Parent string
}

func (f Filter) matches(t *ticket.Ticket) bool {
	if f.Status != "" && t.Status != f.Status {
		return false
	}
	if f.Type != "" && t.Prefix() != strings.ToUpper(f.Type) {
		return false
	}
	if f.Parent != "" && t.Parent != f.Parent {
		return false
	}
	if f.Label != "" {
		found := false
		for _, l := range t.Labels {
			if l == f.Label {
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

// List returns tickets matching the filter, sorted by ID for determinism.
func (s *Store) List(f Filter) []*ticket.Ticket {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*ticket.Ticket
	for _, t := range s.cache {
		if f.matches(t) {
			out = append(out, cloneTicket(t))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// All returns every ticket, sorted by ID.
func (s *Store) All() []*ticket.Ticket { return s.List(Filter{}) }

// Graph builds a dependency/epic graph over the current ticket set.
func (s *Store) Graph() *ticket.Graph {
	return ticket.NewGraph(s.All())
}

// Get returns a copy of a ticket by ID.
func (s *Store) Get(id string) (*ticket.Ticket, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.cache[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneTicket(t), nil
}

// Hash returns the current content hash of a ticket (for If-Match).
func (s *Store) Hash(id string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, ok := s.hash[id]
	return h, ok
}

// SortByPriorityThenUpdated orders tickets by priority (desc) then most recently
// updated. Used by the board and `pine ready`.
func (s *Store) SortByPriorityThenUpdated(ts []*ticket.Ticket) {
	prios := s.Config().Priorities
	sort.SliceStable(ts, func(i, j int) bool {
		ri := ticket.PriorityRank(ts[i].Priority, prios)
		rj := ticket.PriorityRank(ts[j].Priority, prios)
		if ri != rj {
			return ri > rj
		}
		return ts[i].Updated.After(ts[j].Updated)
	})
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func cloneTicket(t *ticket.Ticket) *ticket.Ticket {
	c := *t
	c.Labels = append([]string(nil), t.Labels...)
	c.Deps = append([]string(nil), t.Deps...)
	c.Extra = append([]ticket.ExtraField(nil), t.Extra...)
	c.Warnings = append([]string(nil), t.Warnings...)
	return &c
}

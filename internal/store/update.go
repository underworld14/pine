package store

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/underworld14/pine/internal/config"
	"github.com/underworld14/pine/internal/ticket"
)

// Update applies mut to a copy of the ticket, bumps its updated timestamp, and
// writes it atomically. Degraded tickets are rejected (their frontmatter is not
// understood, so a rewrite would lose data). mut runs under the write lock.
func (s *Store) Update(id string, mut func(*ticket.Ticket) error) (*ticket.Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cur, ok := s.cache[id]
	if !ok {
		return nil, ErrNotFound
	}
	if cur.Degraded {
		return nil, ErrDegraded
	}
	t := cloneTicket(cur)
	if err := mut(t); err != nil {
		return nil, err
	}
	t.Updated = s.now().UTC()
	if err := s.saveTicket(t); err != nil {
		return nil, err
	}
	return cloneTicket(t), nil
}

// Delete removes a ticket file and its attachments directory.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.cache[id]; !ok {
		return ErrNotFound
	}
	if err := os.Remove(s.ticketPath(id)); err != nil && !os.IsNotExist(err) {
		return err
	}
	_ = os.RemoveAll(s.attachmentDir(id))
	delete(s.cache, id)
	delete(s.hash, id)
	return nil
}

// Change describes the effect of reloading a ticket file from disk.
type Change struct {
	ID      string
	Removed bool // the file no longer exists and was dropped from the cache
	Changed bool // the cache/index was updated (content actually differed)
}

// ReloadTicket re-reads one ticket file after an external change (watcher entry
// point). It dedupes by content hash so a write echoed by the filesystem does
// not report a spurious change.
func (s *Store) ReloadTicket(path string) (Change, error) {
	filename := filepath.Base(path)
	if !ticketFileRe.MatchString(filename) {
		return Change{}, nil
	}
	id := strings.TrimSuffix(filename, ".md")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			s.mu.Lock()
			_, existed := s.cache[id]
			delete(s.cache, id)
			delete(s.hash, id)
			s.mu.Unlock()
			return Change{ID: id, Removed: existed, Changed: existed}, nil
		}
		return Change{ID: id}, err
	}

	h := hashBytes(data)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hash[id] == h {
		return Change{ID: id}, nil // no real change
	}
	t := ticket.Parse(id, data)
	applyMtimeFallback(t, path)
	s.cache[id] = t
	s.hash[id] = h
	return Change{ID: id, Changed: true}, nil
}

// ReloadConfig re-reads config.json; returns whether it changed.
func (s *Store) ReloadConfig() (bool, error) {
	cfg, err := config.Load(filepath.Join(s.root, fileConfig))
	if err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	oldB, _ := s.cfg.Bytes()
	newB, _ := cfg.Bytes()
	if string(oldB) == string(newB) {
		return false, nil
	}
	s.cfg = cfg
	return true, nil
}

// ReloadBoard re-reads board.json; returns whether it changed.
func (s *Store) ReloadBoard() (bool, error) {
	board, err := config.LoadBoard(filepath.Join(s.root, fileBoard))
	if err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	oldB, _ := s.board.Bytes()
	newB, _ := board.Bytes()
	if string(oldB) == string(newB) {
		return false, nil
	}
	s.board = board
	return true, nil
}

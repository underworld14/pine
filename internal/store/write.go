package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/underworld14/pine/internal/config"
	"github.com/underworld14/pine/internal/ticket"
)

// saveTicket serializes a ticket, writes it atomically, and refreshes the cache
// and hash. Callers must hold the write lock. Status and priority are normalized
// to lowercase so API/CLI writes match how disk-parsed tickets are stored
// (otherwise "Done" would never equal StatusDone in the dependency graph).
func (s *Store) saveTicket(t *ticket.Ticket) error {
	t.Status = strings.ToLower(strings.TrimSpace(t.Status))
	t.Priority = strings.ToLower(strings.TrimSpace(t.Priority))
	data := t.Serialize()
	if err := atomicWrite(s.ticketPath(t.ID), data); err != nil {
		return err
	}
	s.cache[t.ID] = cloneTicket(t)
	s.hash[t.ID] = hashBytes(data)
	return nil
}

// atomicWrite writes data to path via a same-directory temp file plus rename,
// so readers and the watcher never observe a partially written file. The temp
// name is dot-prefixed (".tmp-…") so the watcher ignores it.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return renameWithRetry(tmpName, path)
}

// renameWithRetry works around transient sharing-violation failures on Windows
// where the destination may be briefly open by an editor or indexer.
func renameWithRetry(from, to string) error {
	var err error
	for i := 0; i < 5; i++ {
		if err = os.Rename(from, to); err == nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return err
}

// SaveConfig validates and atomically writes a new configuration, updating the
// in-memory copy on success.
func (s *Store) SaveConfig(c *config.Config) error {
	if problems := c.Validate(); len(problems) > 0 {
		return fmt.Errorf("invalid config: %s", strings.Join(problems, "; "))
	}
	data, err := c.Bytes()
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := atomicWrite(filepath.Join(s.root, fileConfig), data); err != nil {
		return err
	}
	s.cfg = c
	return nil
}

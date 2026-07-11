package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/underworld14/pine/internal/ticket"
)

// Built-in body templates, used when no templates/<type>.md file exists. They
// begin with a newline so the file has a blank line after the frontmatter.
const (
	bugTemplate     = "\n# Description\n\n# Steps\n\n# Expected\n\n# Actual\n\n# Acceptance Criteria\n- [ ] Define acceptance criteria\n\n# Related Files\n\n# Attachments\n"
	featureTemplate = "\n# Description\n\n# Acceptance Criteria\n- [ ] Define acceptance criteria\n\n# Implementation Plan\n\n# Notes\n\n# Related Files\n\n# Attachments\n"
	epicTemplate    = "\n# Description\n\n# Goals\n\n# Child Tickets\n"
)

// CreateReq describes a new ticket. Type may be an ID prefix ("BUG") or a type
// name ("Bug"). Empty Priority defaults to medium, empty Status to the first
// board column, empty Body to the type's template.
type CreateReq struct {
	Type     string
	Title    string
	Priority string
	Labels   []string
	Deps     []string
	Parent   string
	Status   string
	Body     string
}

// Create allocates a new ticket ID, writes the file atomically, and returns the
// created ticket. ID allocation is race-safe within the process (write lock)
// and across processes (O_EXCL reservation).
func (s *Store) Create(req CreateReq) (*ticket.Ticket, error) {
	if strings.TrimSpace(req.Title) == "" {
		return nil, errors.New("title is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	prefix, ok := s.resolvePrefix(req.Type)
	if !ok {
		return nil, ErrUnknownType
	}
	status := req.Status
	if status == "" {
		status = s.board.FirstStatus()
	}
	priority := req.Priority
	if priority == "" {
		priority = "medium"
	}
	id, err := s.allocID(prefix)
	if err != nil {
		return nil, err
	}
	body := req.Body
	if strings.TrimSpace(body) == "" {
		body = s.template(prefix)
	} else if !strings.HasPrefix(body, "\n") {
		body = "\n" + body
	}
	now := s.now().UTC()
	t := &ticket.Ticket{
		ID:       id,
		Title:    req.Title,
		Status:   status,
		Priority: priority,
		Labels:   req.Labels,
		Deps:     req.Deps,
		Parent:   req.Parent,
		Created:  now,
		Updated:  now,
		Body:     body,
	}
	if err := s.saveTicket(t); err != nil {
		// Clean up the empty O_EXCL reservation so a failed write does not leave a
		// zero-byte file that would parse as a degraded ticket.
		_ = os.Remove(s.ticketPath(id))
		return nil, err
	}
	return cloneTicket(t), nil
}

// resolvePrefix maps a user-supplied type token to a configured ID prefix.
func (s *Store) resolvePrefix(typ string) (string, bool) {
	up := strings.ToUpper(strings.TrimSpace(typ))
	if _, ok := s.cfg.TypeByPrefix(up); ok {
		return up, true
	}
	for _, t := range s.cfg.Types {
		if strings.EqualFold(t.Name, typ) {
			return t.Prefix, true
		}
	}
	return "", false
}

// allocID reserves a free ticket ID for the prefix (the empty reservation is
// replaced by saveTicket's atomic rename).
func (s *Store) allocID(prefix string) (string, error) {
	return s.allocIDIn(s.ticketsDir(), s.ticketPath, prefix, "ticket")
}

// allocIDIn reserves a free ID for prefix inside dir, using pathFor to build
// each candidate's reservation path with an exclusive create. kind names the
// entity for error messages only ("ticket", "learning"). Hash style (default)
// generates a random suffix — no directory scan and no cross-branch
// collisions; sequential style scans dir for the highest existing number.
func (s *Store) allocIDIn(dir string, pathFor func(id string) string, prefix, kind string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if s.cfg.IDStyle == "sequential" {
		return s.allocSequentialIn(dir, pathFor, prefix, kind)
	}
	return s.allocHashIn(pathFor, prefix, kind)
}

func (s *Store) allocHashIn(pathFor func(id string) string, prefix, kind string) (string, error) {
	for i := 0; i < 20; i++ {
		id := ticket.MakeID(prefix, s.idSuffix())
		f, err := os.OpenFile(pathFor(id), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			f.Close()
			return id, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", err
		}
	}
	return "", errors.New("could not allocate a unique " + kind + " id")
}

func (s *Store) allocSequentialIn(dir string, pathFor func(id string) string, prefix, kind string) (string, error) {
	max := 0
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			name := strings.TrimSuffix(e.Name(), ".md")
			if p, n, err := ticket.SplitID(name); err == nil && p == prefix && n > max {
				max = n
			}
		}
	}
	for i := 1; i <= 500; i++ {
		id := ticket.FormatID(prefix, max+i)
		f, err := os.OpenFile(pathFor(id), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			f.Close()
			return id, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", err
		}
	}
	return "", errors.New("could not allocate a " + kind + " id")
}

// template returns the body skeleton for a ticket type: a templates/<name>.md
// file when present, else a built-in default.
func (s *Store) template(prefix string) string {
	name := strings.ToLower(prefix)
	if typ, ok := s.cfg.TypeByPrefix(prefix); ok {
		name = strings.ToLower(typ.Name)
	}
	if data, err := os.ReadFile(filepath.Join(s.root, dirTemplates, name+".md")); err == nil {
		body := string(data)
		if !strings.HasPrefix(body, "\n") {
			body = "\n" + body
		}
		return body
	}
	switch prefix {
	case "EPIC":
		return epicTemplate
	case "BUG":
		return bugTemplate
	default:
		return featureTemplate
	}
}

package server

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/underworld14/pine/internal/store"
)

// StoreRegistry holds one or more open .pine stores for multi-repo serve.
// Repos register themselves on `pine init` into ~/.pine/repos.json, and
// `pine serve` auto-discovers them — no workspace name or flag is required.
type StoreRegistry struct {
	mu     sync.RWMutex
	stores map[string]*store.Store
	order  []string
	active string
	paths  map[string]string // alias -> repo root (parent of .pine)
}

// NewRegistry builds a registry from alias -> store. The first alias in order becomes active.
func NewRegistry(ordered []string, stores map[string]*store.Store, paths map[string]string) (*StoreRegistry, error) {
	if len(ordered) == 0 {
		return nil, fmt.Errorf("registry requires at least one store")
	}
	for _, a := range ordered {
		if stores[a] == nil {
			return nil, fmt.Errorf("missing store for alias %q", a)
		}
	}
	if paths == nil {
		paths = map[string]string{}
	}
	return &StoreRegistry{
		stores: stores,
		order:  append([]string(nil), ordered...),
		active: ordered[0],
		paths:  paths,
	}, nil
}

// SingleRegistry wraps one store under alias "default".
func SingleRegistry(st *store.Store) *StoreRegistry {
	alias := "default"
	root := ""
	if st != nil {
		root = filepath.Dir(st.Root())
	}
	reg, _ := NewRegistry([]string{alias}, map[string]*store.Store{alias: st}, map[string]string{alias: root})
	return reg
}

// IsWorkspace reports whether more than one repo is registered. The UI uses
// this to decide whether to render the repo switcher.
func (r *StoreRegistry) IsWorkspace() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.order) > 1
}

func (r *StoreRegistry) ActiveAlias() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.active
}

func (r *StoreRegistry) Active() *store.Store {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.stores[r.active]
}

func (r *StoreRegistry) Get(alias string) (*store.Store, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	st, ok := r.stores[alias]
	return st, ok
}

func (r *StoreRegistry) SetActive(alias string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.stores[alias]; !ok {
		return fmt.Errorf("unknown repo alias %q", alias)
	}
	r.active = alias
	return nil
}

// All returns aliases in registration order.
func (r *StoreRegistry) All() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]string(nil), r.order...)
}

// Entries returns alias, project name, and repo path for each store.
func (r *StoreRegistry) Entries() []map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]map[string]string, 0, len(r.order))
	for _, a := range r.order {
		st := r.stores[a]
		project := ""
		repo := r.paths[a]
		if st != nil {
			project = st.Config().Project.Name
			if repo == "" {
				repo = st.Root()
			}
		}
		out = append(out, map[string]string{
			"alias":   a,
			"project": project,
			"repo":    repo,
			"pine":    st.Root(),
		})
	}
	return out
}

func (r *StoreRegistry) AliasOf(st *store.Store) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for a, s := range r.stores {
		if s == st {
			return a
		}
	}
	return r.active
}

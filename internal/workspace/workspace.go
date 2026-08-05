// Package workspace manages the machine-wide registry of Pine repos at
// ~/.pine/repos.json. Repos register themselves on `pine init` and `pine serve`
// auto-discovers them — no separate workspace command or flag.
package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/underworld14/pine/internal/memory"
)

// RegistryFile is the on-disk registry path under the Pine home.
const RegistryFile = "repos.json"

// Repo is one registered repository.
type Repo struct {
	Path       string    `json:"path"`       // absolute path to the repo root (parent of .pine)
	Alias      string    `json:"alias"`      // display + routing alias (default: lowercase basename)
	Registered time.Time `json:"registered"`
}

// Registry is the on-disk JSON shape.
type Registry struct {
	Repos []Repo `json:"repos"`
}

// registryPath returns ~/.pine/repos.json (or $PINE_HOME/repos.json).
func registryPath() (string, error) {
	home, err := memory.GlobalDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, RegistryFile), nil
}

// Load reads the registry. A missing file yields an empty registry (no error).
func Load() (*Registry, error) {
	path, err := registryPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Registry{}, nil
		}
		return nil, err
	}
	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("registry %s: %w", path, err)
	}
	return &reg, nil
}

// Save writes the registry atomically, creating the home dir if needed.
func (r *Registry) Save() error {
	path, err := registryPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Register adds a repo (by absolute path) to the registry. Alias defaults to the
// lowercase basename of the path. Returns the registry entry. Idempotent:
// re-registering an existing path updates only the alias when alias != "".
func (r *Registry) Register(absPath, alias string) (Repo, error) {
	absPath = filepath.Clean(absPath)
	if !filepath.IsAbs(absPath) {
		return Repo{}, fmt.Errorf("path must be absolute, got %q", absPath)
	}
	if alias == "" {
		alias = defaultAlias(absPath)
	}
	if err := validateAlias(alias); err != nil {
		return Repo{}, err
	}
	for i, re := range r.Repos {
		if re.Path == absPath {
			if alias != "" && re.Alias != alias {
				r.Repos[i].Alias = alias
				return r.Repos[i], nil
			}
			return re, nil
		}
		// Reject a duplicate alias pointing at a different path.
		if re.Alias == alias {
			return Repo{}, fmt.Errorf("alias %q already registered for %s", alias, re.Path)
		}
	}
	entry := Repo{Path: absPath, Alias: alias, Registered: time.Now().UTC()}
	r.Repos = append(r.Repos, entry)
	return entry, nil
}

// Unregister removes a repo by path. Returns ErrNotRegistered if absent.
func (r *Registry) Unregister(absPath string) error {
	absPath = filepath.Clean(absPath)
	for i, re := range r.Repos {
		if re.Path == absPath {
			r.Repos = append(r.Repos[:i], r.Repos[i+1:]...)
			return nil
		}
	}
	return ErrNotRegistered
}

// Find returns the repo entry for a path, or nil.
func (r *Registry) Find(absPath string) *Repo {
	absPath = filepath.Clean(absPath)
	for i, re := range r.Repos {
		if re.Path == absPath {
			return &r.Repos[i]
		}
	}
	return nil
}

// FindAlias returns the repo entry for an alias, or nil.
func (r *Registry) FindAlias(alias string) *Repo {
	for i, re := range r.Repos {
		if re.Alias == alias {
			return &r.Repos[i]
		}
	}
	return nil
}

// Sorted returns repos sorted by alias.
func (r *Registry) Sorted() []Repo {
	out := append([]Repo(nil), r.Repos...)
	sort.Slice(out, func(i, j int) bool { return out[i].Alias < out[j].Alias })
	return out
}

// ErrNotRegistered is returned by Unregister when the path is not in the registry.
var ErrNotRegistered = fmt.Errorf("repo is not registered")

// RegisterPath is a convenience: load, register, save. Returns the entry.
func RegisterPath(absPath, alias string) (Repo, error) {
	reg, err := Load()
	if err != nil {
		return Repo{}, err
	}
	entry, err := reg.Register(absPath, alias)
	if err != nil {
		return Repo{}, err
	}
	if err := reg.Save(); err != nil {
		return Repo{}, err
	}
	return entry, nil
}

// UnregisterPath is a convenience: load, remove, save.
func UnregisterPath(absPath string) error {
	reg, err := Load()
	if err != nil {
		return err
	}
	if err := reg.Unregister(absPath); err != nil {
		return err
	}
	return reg.Save()
}

// ListAll is a convenience: load + return sorted repos.
func ListAll() ([]Repo, error) {
	reg, err := Load()
	if err != nil {
		return nil, err
	}
	return reg.Sorted(), nil
}

// ResolvePineDir finds the .pine directory for a repo path (the path itself or a child .pine).
func ResolvePineDir(repoPath string) (string, error) {
	repoPath = filepath.Clean(repoPath)
	candidates := []string{
		filepath.Join(repoPath, ".pine"),
		repoPath,
	}
	for _, c := range candidates {
		fi, err := os.Stat(c)
		if err != nil || !fi.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(c, "config.json")); err == nil {
			return c, nil
		}
		if filepath.Base(c) == ".pine" {
			return c, nil
		}
	}
	return "", fmt.Errorf("no .pine store found under %q (run pine init there first)", repoPath)
}

func defaultAlias(absPath string) string {
	return strings.ToLower(filepath.Base(absPath))
}

// validateAlias rejects aliases that would break routing or collide with
// path segments: empty, containing slashes, or with characters outside
// [a-z0-9_-].
func validateAlias(alias string) error {
	if alias == "" {
		return fmt.Errorf("alias must not be empty")
	}
	if strings.ContainsAny(alias, "/\\") {
		return fmt.Errorf("alias %q must not contain slashes", alias)
	}
	for _, r := range alias {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return fmt.Errorf("alias %q may only contain a-z, 0-9, - or _", alias)
		}
	}
	return nil
}

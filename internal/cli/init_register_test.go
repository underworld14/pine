package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/underworld14/pine/internal/memory"
	"github.com/underworld14/pine/internal/workspace"
)

// TestInitRegistersRepo verifies `pine init` writes the repo into the
// machine-wide registry at $PINE_HOME/repos.json so `pine serve` and the
// dashboard can see it alongside other projects.
func TestInitRegistersRepo(t *testing.T) {
	home := t.TempDir()
	t.Setenv(memory.EnvHome, home)

	repo := initRepo(t)
	reg, err := workspace.Load()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	if len(reg.Repos) != 1 {
		t.Fatalf("expected 1 registered repo, got %d: %+v", len(reg.Repos), reg.Repos)
	}
	entry := reg.Repos[0]
	if entry.Path != repo {
		t.Errorf("registered path = %q want %q", entry.Path, repo)
	}
	if entry.Alias != filepath.Base(repo) {
		t.Errorf("alias = %q want %q", entry.Alias, filepath.Base(repo))
	}

	// Re-running init is idempotent: no duplicate entry.
	if _, err := run(t, repo, "init", "--skip-agents"); err != nil {
		t.Fatalf("re-init: %v", err)
	}
	reg, _ = workspace.Load()
	if len(reg.Repos) != 1 {
		t.Fatalf("after re-init: expected 1 repo, got %d", len(reg.Repos))
	}
}

// TestServeAutoDiscoversRegistry verifies `pine serve` opens every repo listed
// in $PINE_HOME/repos.json without a --workspace flag, with the cwd repo active.
func TestServeAutoDiscoversRegistry(t *testing.T) {
	home := t.TempDir()
	t.Setenv(memory.EnvHome, home)

	api := initRepoNamed(t, "api")
	web := initRepoNamed(t, "web")

	// Both repos should be registered by their init calls.
	reg, _ := workspace.Load()
	if len(reg.Repos) != 2 {
		t.Fatalf("expected 2 registered repos, got %d", len(reg.Repos))
	}
	if reg.Find(web) == nil {
		t.Errorf("web repo path %q not registered", web)
	}

	// Start serve from the api repo and inspect /api/repos via direct registry
	// build (we exercise buildServeRegistry rather than a full HTTP server, to
	// avoid binding a port in tests).
	origDir := flagDir
	t.Cleanup(func() { flagDir = origDir })

	flagDir = api
	built, err := buildServeRegistry(false)
	if err != nil {
		t.Fatalf("buildServeRegistry from api: %v", err)
	}
	if !built.IsWorkspace() {
		t.Fatalf("expected workspace mode with 2 repos, got aliases %v", built.All())
	}
	if built.ActiveAlias() != "api" {
		t.Errorf("active = %q want api", built.ActiveAlias())
	}
	if len(built.All()) != 2 {
		t.Errorf("aliases = %v want [api web]", built.All())
	}

	// --only restricts to the cwd repo.
	flagDir = api
	only, err := buildServeRegistry(true)
	if err != nil {
		t.Fatalf("buildServeRegistry --only: %v", err)
	}
	if only.IsWorkspace() {
		t.Errorf("--only should yield a single-repo registry, got %v", only.All())
	}
}

// initRepoNamed creates a repo dir with the given basename and runs pine init there.
func initRepoNamed(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, dir, "init", "--skip-agents"); err != nil {
		t.Fatalf("init %s: %v", name, err)
	}
	// Pin sequential IDs for stable assertions.
	cfgPath := filepath.Join(dir, ".pine", "config.json")
	if raw, err := os.ReadFile(cfgPath); err == nil {
		patched := strings.ReplaceAll(string(raw), `"idStyle":"hash"`, `"idStyle":"sequential"`)
		os.WriteFile(cfgPath, []byte(patched), 0o644)
	}
	return dir
}

func TestRegistryFileShape(t *testing.T) {
	home := t.TempDir()
	t.Setenv(memory.EnvHome, home)
	initRepoNamed(t, "alpha")

	data, err := os.ReadFile(filepath.Join(home, workspace.RegistryFile))
	if err != nil {
		t.Fatal(err)
	}
	var reg workspace.Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		t.Fatalf("repos.json is not valid JSON: %v", err)
	}
	if len(reg.Repos) != 1 || reg.Repos[0].Alias != "alpha" {
		t.Errorf("repos.json shape = %+v", reg)
	}
}

// TestInitAliasFlag verifies `pine init --alias <name>` registers the repo
// under the chosen alias instead of the default lowercase basename — so two
// repos with the same basename don't silently collide.
func TestInitAliasFlag(t *testing.T) {
	home := t.TempDir()
	t.Setenv(memory.EnvHome, home)

	dir := filepath.Join(t.TempDir(), "web")
	os.MkdirAll(dir, 0o755)
	if _, err := run(t, dir, "init", "--skip-agents", "--alias", "web-frontend"); err != nil {
		t.Fatalf("init --alias: %v", err)
	}
	reg, _ := workspace.Load()
	if len(reg.Repos) != 1 || reg.Repos[0].Alias != "web-frontend" {
		t.Fatalf("expected alias web-frontend, got %+v", reg.Repos)
	}

	// A second repo with the same default basename would collide; --alias
	// resolves it.
	dir2 := filepath.Join(t.TempDir(), "web")
	os.MkdirAll(dir2, 0o755)
	if _, err := run(t, dir2, "init", "--skip-agents", "--alias", "web-backend"); err != nil {
		t.Fatalf("init --alias (second): %v", err)
	}
	reg, _ = workspace.Load()
	if len(reg.Repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(reg.Repos))
	}
}

// TestServeOnlyDoesNotWriteRegistry verifies `pine serve --only` is read-only on
// the registry: an unregistered cwd repo is NOT auto-registered (review M3).
func TestServeOnlyDoesNotWriteRegistry(t *testing.T) {
	home := t.TempDir()
	t.Setenv(memory.EnvHome, home)

	repo := initRepo(t) // registers 1 repo
	if err := workspace.UnregisterPath(repo); err != nil {
		t.Fatalf("unregister: %v", err)
	}
	// Registry is now empty; the cwd repo is unregistered.
	reg, _ := workspace.Load()
	if len(reg.Repos) != 0 {
		t.Fatalf("expected empty registry, got %d", len(reg.Repos))
	}

	origDir := flagDir
	t.Cleanup(func() { flagDir = origDir })
	flagDir = repo
	if _, err := buildServeRegistry(true); err != nil {
		t.Fatalf("buildServeRegistry --only: %v", err)
	}
	// --only must NOT have re-registered the cwd repo.
	reg, _ = workspace.Load()
	if len(reg.Repos) != 0 {
		t.Fatalf("--only wrote the registry: %d repos after serve", len(reg.Repos))
	}
}

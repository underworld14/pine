package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegisterAndSave(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PINE_HOME", home)

	api := filepath.Join(home, "api")
	web := filepath.Join(home, "web")

	entry, err := RegisterPath(api, "")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Alias != "api" {
		t.Errorf("alias = %q want api", entry.Alias)
	}
	if _, err := RegisterPath(web, "web"); err != nil {
		t.Fatal(err)
	}

	reg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(reg.Repos) != 2 {
		t.Fatalf("repos = %d", len(reg.Repos))
	}

	// Idempotent: re-register api with no alias change.
	if _, err := RegisterPath(api, ""); err != nil {
		t.Fatal(err)
	}
	if len(reg.Repos) != 2 {
		t.Fatalf("repos = %d after re-register", len(reg.Repos))
	}

	// Duplicate alias to a different path is rejected.
	if _, err := RegisterPath(filepath.Join(home, "other"), "api"); err == nil {
		t.Fatal("expected duplicate alias error")
	}

	// Unregister.
	if err := UnregisterPath(web); err != nil {
		t.Fatal(err)
	}
	reg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(reg.Repos) != 1 || reg.Repos[0].Alias != "api" {
		t.Fatalf("after unregister = %+v", reg.Repos)
	}
}

func TestRegisterRejectsRelative(t *testing.T) {
	t.Setenv("PINE_HOME", t.TempDir())
	if _, err := RegisterPath("relative/path", ""); err == nil {
		t.Fatal("expected error for relative path")
	}
}

func TestResolvePineDir(t *testing.T) {
	root := t.TempDir()
	pine := filepath.Join(root, ".pine")
	if err := os.MkdirAll(pine, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pine, "config.json"), []byte(`{"version":1,"project":{"name":"t"},"types":[{"prefix":"BUG","name":"Bug"}],"priorities":["low","medium","high","critical"],"attachments":{},"git":{},"idStyle":"hash","crossBranch":{},"sync":{},"context":{"globalMemory":true}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ResolvePineDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != pine {
		t.Fatalf("got %s want %s", got, pine)
	}
}

// TestRegistryFindSortedValidate covers the read-side helpers (Find, FindAlias,
// Sorted, ListAll) and the validateAlias error paths that the register test
// doesn't reach.
func TestRegistryFindSortedValidate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PINE_HOME", home)

	api := filepath.Join(home, "api")
	web := filepath.Join(home, "web")
	if _, err := RegisterPath(api, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := RegisterPath(web, "web"); err != nil {
		t.Fatal(err)
	}

	// ListAll returns sorted-by-alias repos.
	all, err := ListAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].Alias != "api" || all[1].Alias != "web" {
		t.Fatalf("ListAll = %+v", all)
	}

	reg, _ := Load()
	if got := reg.Sorted(); len(got) != 2 || got[0].Alias != "api" {
		t.Fatalf("Sorted = %+v", got)
	}
	if reg.Find(api) == nil {
		t.Error("Find(api) nil")
	}
	if reg.Find(filepath.Join(home, "missing")) != nil {
		t.Error("Find(missing) should be nil")
	}
	if reg.FindAlias("web") == nil {
		t.Error("FindAlias(web) nil")
	}
	if reg.FindAlias("nope") != nil {
		t.Error("FindAlias(nope) should be nil")
	}

	// validateAlias error paths. Register fills an empty alias with the default
	// before validating, so "" is exercised directly; the rest go via Register.
	if err := validateAlias(""); err == nil {
		t.Error("validateAlias(\"\") should fail")
	}
	for _, bad := range []string{"has/slash", "has\\back", "UPPER", "with space", "dot.x"} {
		if _, err := reg.Register(home, bad); err == nil {
			t.Errorf("Register alias %q should fail", bad)
		}
	}

	// Re-registering an existing path with a new alias updates it in place.
	if _, err := reg.Register(api, "api-2"); err != nil {
		t.Fatal(err)
	}
	if got := reg.Find(api); got == nil || got.Alias != "api-2" {
		t.Fatalf("alias update = %+v", got)
	}

	// Unregister on a missing path returns ErrNotRegistered.
	if err := reg.Unregister(filepath.Join(home, "missing")); err != ErrNotRegistered {
		t.Fatalf("Unregister(missing) = %v, want ErrNotRegistered", err)
	}

	// ResolvePineDir on a dir with no .pine returns an error.
	if _, err := ResolvePineDir(home); err == nil {
		t.Fatal("ResolvePineDir on a dir with no .pine should error")
	}
}

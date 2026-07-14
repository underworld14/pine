package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain pins PINE_HOME to a throwaway dir for the whole package, so a test
// that forgets its own t.Setenv can never reach the developer's real ~/.pine.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "pine-home-")
	if err != nil {
		panic(err)
	}
	os.Setenv(EnvHome, dir)
	code := m.Run()
	// Must precede os.Exit — a defer would never run.
	os.RemoveAll(dir)
	os.Exit(code)
}

func TestResolveGlobalDir(t *testing.T) {
	cases := []struct {
		pineHome, userHome, want string
		wantErr                  bool
	}{
		{"/tmp/explicit", "/home/u", "/tmp/explicit", false},
		{"/tmp/explicit/", "/home/u", "/tmp/explicit", false},          // trailing sep cleaned
		{"/tmp/a/../explicit", "/home/u", "/tmp/explicit", false},      // cleaned
		{"", "/home/u", filepath.Join("/home/u", DirGlobal), false},
		{"   ", "/home/u", filepath.Join("/home/u", DirGlobal), false}, // blank ≠ set
		{"rel-pine", "/home/u", "", true},                              // relative → rejected, not cwd-resolved
		{"./rel", "/home/u", "", true},
		{"", "", "", true}, // no home, no PINE_HOME
		{"", "   ", "", true},
	}
	for _, c := range cases {
		got, err := resolveGlobalDir(c.pineHome, c.userHome)
		if c.wantErr {
			if err == nil {
				t.Errorf("resolveGlobalDir(%q,%q) = %q, want error", c.pineHome, c.userHome, got)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("resolveGlobalDir(%q,%q) = (%q,%v), want %q", c.pineHome, c.userHome, got, err, c.want)
		}
	}
}

func TestResolveGlobalDirErrorNamesPineHome(t *testing.T) {
	_, err := resolveGlobalDir("", "")
	if err == nil || !strings.Contains(err.Error(), EnvHome) {
		t.Fatalf("error should tell the user about %s, got: %v", EnvHome, err)
	}
}

func TestGlobalDirReadsEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvHome, dir)
	got, err := GlobalDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Errorf("GlobalDir() = %q, want %q", got, dir)
	}
}

func TestGlobalDirCreatesNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "absent")
	t.Setenv(EnvHome, dir)
	if _, err := GlobalDir(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("GlobalDir must not create the store")
	}
}

func TestGlobalLabelDefaultAndRelocated(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	if got := GlobalLabel(filepath.Join(home, DirGlobal)); got != "~/"+DirGlobal {
		t.Errorf("default location should render as ~/%s, got %q", DirGlobal, got)
	}
	// Under PINE_HOME a tilde path would name a file that does not exist.
	if got := GlobalLabel("/tmp/relocated"); got != "/tmp/relocated" {
		t.Errorf("relocated store should render as its path, got %q", got)
	}
}

func TestEnsureGlobalLayoutSeedsPersonalMEMORY(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvHome, dir)
	got, err := EnsureGlobalLayout()
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("EnsureGlobalLayout() = %q, want %q", got, dir)
	}
	body, err := ReadMEMORY(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# Personal memory", "every repository", "## Preferences", "## Log"} {
		if !strings.Contains(body, want) {
			t.Errorf("global seed missing %q in:\n%s", want, body)
		}
	}
	if strings.Contains(body, "for this repository") {
		t.Errorf("global seed must not use the project wording:\n%s", body)
	}
	if _, err := os.Stat(TopicsDir(dir)); err != nil {
		t.Errorf("memory/ not created: %v", err)
	}
}

func TestEnsureGlobalLayoutKeepsExistingMEMORY(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvHome, dir)
	if _, err := EnsureGlobalLayout(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(MemoryPath(dir), []byte("# Mine\n\n## Log\n- sentinel\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureGlobalLayout(); err != nil {
		t.Fatal(err)
	}
	body, _ := ReadMEMORY(dir)
	if !strings.Contains(body, "sentinel") {
		t.Errorf("existing global MEMORY.md was clobbered:\n%s", body)
	}
}

func TestEnsureLayoutStillSeedsProjectMEMORY(t *testing.T) {
	// Regression: the project seed must be unaffected by the global store,
	// even with PINE_HOME pointing elsewhere.
	t.Setenv(EnvHome, t.TempDir())
	pine := filepath.Join(t.TempDir(), ".pine")
	if err := EnsureLayout(pine); err != nil {
		t.Fatal(err)
	}
	body, _ := ReadMEMORY(pine)
	if !strings.Contains(body, "# Project memory") {
		t.Errorf("project seed changed:\n%s", body)
	}
}

func TestAppendMEMORYAfterEnsureGlobalKeepsPersonalSeed(t *testing.T) {
	// The ordering invariant: AppendMEMORY calls EnsureLayout (project seed)
	// internally, so it must find the personal seed already in place.
	dir := t.TempDir()
	t.Setenv(EnvHome, dir)
	if _, err := EnsureGlobalLayout(); err != nil {
		t.Fatal(err)
	}
	if err := AppendMEMORY(dir, AppendOpts{Text: "I use pnpm, never npm"}); err != nil {
		t.Fatal(err)
	}
	body, _ := ReadMEMORY(dir)
	if !strings.Contains(body, "I use pnpm, never npm") {
		t.Errorf("bullet not appended:\n%s", body)
	}
	if !strings.Contains(body, "# Personal memory") {
		t.Errorf("inner EnsureLayout re-seeded with project text:\n%s", body)
	}
	if strings.Contains(body, "# Project memory") {
		t.Errorf("project seed leaked into the global store:\n%s", body)
	}
}

func TestSuggestDoesNotCreateLayout(t *testing.T) {
	// `pine learn suggest` is documented as "no write".
	dir := t.TempDir()
	if _, err := Suggest(dir, SuggestOpts{Text: "some insight"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(MemoryPath(dir)); !os.IsNotExist(err) {
		t.Errorf("Suggest created MEMORY.md")
	}
	if _, err := os.Stat(TopicsDir(dir)); !os.IsNotExist(err) {
		t.Errorf("Suggest created memory/")
	}
}

func TestTruncateForContextUsesGivenLabel(t *testing.T) {
	long := strings.Repeat("line\n", 1000)
	got := TruncateForContext(long, 80, "~/.pine/MEMORY.md")
	// Assert the full backticked string: "~/.pine/MEMORY.md" contains
	// ".pine/MEMORY.md" as a substring, so a negative check would be useless.
	if !strings.Contains(got, "see `~/.pine/MEMORY.md`") {
		t.Fatalf("notice should name the given store:\n%s", got)
	}
}

func TestEnsureGlobalLayoutErrorsOnUnwritableHome(t *testing.T) {
	// PINE_HOME points at a regular file → MkdirAll cannot create memory/.
	f := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvHome, f)
	if _, err := EnsureGlobalLayout(); err == nil {
		t.Fatal("expected an error when the global home is not a directory")
	}
}

func TestResolveGlobalDirRejectsRelativePineHome(t *testing.T) {
	// A relative PINE_HOME would resolve against the cwd, giving a different
	// "machine-wide" store per directory and scattering stray dirs into repos.
	_, err := resolveGlobalDir("relhome", "/home/u")
	if err == nil {
		t.Fatal("a relative PINE_HOME must be rejected, not cwd-resolved")
	}
	if !strings.Contains(err.Error(), "absolute") || !strings.Contains(err.Error(), EnvHome) {
		t.Errorf("error should name %s and say absolute, got: %v", EnvHome, err)
	}
}

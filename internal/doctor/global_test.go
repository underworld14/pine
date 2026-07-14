package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/underworld14/pine/internal/config"
	"github.com/underworld14/pine/internal/memory"
)

// TestMain pins PINE_HOME to a throwaway dir for the whole package, so a test
// that forgets its own t.Setenv can never read the developer's real ~/.pine.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "pine-home-")
	if err != nil {
		panic(err)
	}
	os.Setenv(memory.EnvHome, dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// globalFindings runs only checkGlobalMemory and returns its findings.
func globalFindings(t *testing.T, cfg *config.Config) *Report {
	t.Helper()
	r := &Report{}
	checkGlobalMemory(r, cfg)
	return r
}

func seedGlobalMEMORY(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(memory.EnvHome, dir)
	if _, err := memory.EnsureGlobalLayout(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(memory.MemoryPath(dir), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCheckGlobalMemoryOversizeWarns(t *testing.T) {
	seedGlobalMEMORY(t, strings.Repeat("- a long personal preference line\n", 200))
	r := globalFindings(t, config.Default("x"))
	if len(r.Findings) != 1 {
		t.Fatalf("want one finding, got %v", r.Findings)
	}
	f := r.Findings[0]
	if f.Level != LevelWarn {
		t.Errorf("oversize global memory should warn, not error (level %v)", f.Level)
	}
	if !strings.Contains(f.Msg, "pine context injects only the first 2048") {
		t.Errorf("message should name the cap:\n%s", f.Msg)
	}
	if f.Code != "" || f.Fixable() {
		t.Errorf("nothing here is mechanically repairable: code=%q fixable=%v", f.Code, f.Fixable())
	}
}

func TestCheckGlobalMemorySilentWhenMissing(t *testing.T) {
	t.Setenv(memory.EnvHome, filepath.Join(t.TempDir(), "absent"))
	r := globalFindings(t, config.Default("x"))
	if len(r.Findings) != 0 {
		t.Errorf("a missing global store is normal, not a finding: %v", r.Findings)
	}
}

func TestCheckGlobalMemorySilentWhenOptedOut(t *testing.T) {
	seedGlobalMEMORY(t, strings.Repeat("- a long personal preference line\n", 200))
	cfg := config.Default("x")
	cfg.Context.GlobalMemory = false
	r := globalFindings(t, cfg)
	if len(r.Findings) != 0 {
		t.Errorf("opted out → nothing injected → nothing to report: %v", r.Findings)
	}
}

func TestCheckGlobalMemoryOKWhenPresent(t *testing.T) {
	seedGlobalMEMORY(t, "# Personal memory\n\n## Log\n- I use pnpm\n")
	r := globalFindings(t, config.Default("x"))
	if len(r.Findings) != 1 || r.Findings[0].Level != LevelOK {
		t.Fatalf("want one OK finding, got %v", r.Findings)
	}
	if !strings.Contains(r.Findings[0].Msg, "global memory") {
		t.Errorf("message should name the store:\n%s", r.Findings[0].Msg)
	}
}

func TestCheckGlobalMemoryUnreadableWarns(t *testing.T) {
	// PINE_HOME points at a regular file → ReadMEMORY errors.
	f := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(memory.EnvHome, f)
	r := globalFindings(t, config.Default("x"))
	if len(r.Findings) != 1 || r.Findings[0].Level != LevelWarn {
		t.Fatalf("want one warning, got %v", r.Findings)
	}
	if !strings.Contains(r.Findings[0].Msg, "not readable") {
		t.Errorf("message should say why:\n%s", r.Findings[0].Msg)
	}
}

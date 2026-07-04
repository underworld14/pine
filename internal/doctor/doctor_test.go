package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/underworld14/pine/internal/config"
	"github.com/underworld14/pine/internal/store"
)

func scaffold(t *testing.T) (*store.Store, string) {
	t.Helper()
	pine := filepath.Join(t.TempDir(), ".pine")
	if err := os.MkdirAll(filepath.Join(pine, "tickets"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default("test")
	cfg.IDStyle = "sequential"
	cfgB, _ := cfg.Bytes()
	os.WriteFile(filepath.Join(pine, "config.json"), cfgB, 0o644)
	bB, _ := config.DefaultBoard().Bytes()
	os.WriteFile(filepath.Join(pine, "board.json"), bB, 0o644)
	s, err := store.Open(pine)
	if err != nil {
		t.Fatal(err)
	}
	return s, pine
}

func reopen(t *testing.T, pine string) *store.Store {
	t.Helper()
	s, err := store.Open(pine)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func msgs(r *Report) string {
	var b strings.Builder
	for _, f := range r.Findings {
		b.WriteString(f.Msg)
		b.WriteString("\n")
	}
	return b.String()
}

func TestCleanWorkspacePasses(t *testing.T) {
	s, _ := scaffold(t)
	s.Create(store.CreateReq{Type: "bug", Title: "x"})
	r := Run(s)
	if r.HasErrors() {
		t.Errorf("clean workspace should have no errors:\n%s", msgs(r))
	}
}

func TestDetectsCycleAndDangling(t *testing.T) {
	s, pine := scaffold(t)
	s.Create(store.CreateReq{Type: "bug", Title: "a"}) // BUG-001
	s.Create(store.CreateReq{Type: "bug", Title: "b"}) // BUG-002
	writeDeps(t, pine, "BUG-001", []string{"BUG-002"})
	writeDeps(t, pine, "BUG-002", []string{"BUG-001", "GHOST-999"})

	r := Run(reopen(t, pine))
	out := msgs(r)
	if !r.HasErrors() || !strings.Contains(out, "dependency cycle") {
		t.Errorf("expected a cycle error:\n%s", out)
	}
	if !strings.Contains(out, "dangling dependency GHOST-999") {
		t.Errorf("expected dangling dependency warning:\n%s", out)
	}
}

func TestDetectsDegradedAndStray(t *testing.T) {
	_, pine := scaffold(t)
	os.WriteFile(filepath.Join(pine, "tickets", "BUG-001.md"), []byte("no frontmatter here\n"), 0o644)
	os.WriteFile(filepath.Join(pine, "tickets", "notes.txt"), []byte("scratch"), 0o644)

	r := Run(reopen(t, pine))
	out := msgs(r)
	if !r.HasErrors() || !strings.Contains(out, "malformed") {
		t.Errorf("expected malformed error:\n%s", out)
	}
	if !strings.Contains(out, "stray file") {
		t.Errorf("expected stray file warning:\n%s", out)
	}
}

func TestDetectsBrokenAttachmentRef(t *testing.T) {
	s, pine := scaffold(t)
	s.Create(store.CreateReq{Type: "bug", Title: "x",
		Body: "# Attachments\n- ../attachments/BUG-001/missing.webp\n"})
	r := Run(reopen(t, pine))
	if !r.HasErrors() || !strings.Contains(msgs(r), "missing attachment") {
		t.Errorf("expected missing-attachment error:\n%s", msgs(r))
	}
}

func writeDeps(t *testing.T, pine, id string, deps []string) {
	t.Helper()
	path := filepath.Join(pine, "tickets", id+".md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	depBlock := "deps:\n"
	for _, d := range deps {
		depBlock += "  - " + d + "\n"
	}
	// Insert a deps block just before the closing frontmatter delimiter.
	s := string(raw)
	idx := strings.Index(s, "\n---\n")
	if idx < 0 {
		t.Fatalf("no frontmatter in %s", id)
	}
	updated := s[:idx+1] + depBlock + s[idx+1:]
	os.WriteFile(path, []byte(updated), 0o644)
}

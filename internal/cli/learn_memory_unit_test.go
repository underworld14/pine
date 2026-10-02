package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/underworld14/pine/internal/memory"
)

func TestWriteMemoryDestUnknownKind(t *testing.T) {
	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	err := writeMemoryDest(cmd, memStore{Dir: t.TempDir(), Label: ".pine"}, "bogus", "x", memory.AppendOpts{Text: "hi"}, false)
	if err == nil {
		t.Fatal("expected unknown kind")
	}
}

// TestLearnEvidenceFlag checks `pine learn --evidence` records the proof inline
// with the dated bullet in the destination topic file.
func TestLearnEvidenceFlag(t *testing.T) {
	dir := initRepo(t)
	if _, err := run(t, dir, "learn", "gateway honors platform_toolsets",
		"--new-topic", "agent-api", "--evidence", "probe: curl /api/toolsets shows merged set"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".pine", "memory", "agent-api.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "(evidence: probe: curl /api/toolsets shows merged set)") {
		t.Fatalf("evidence missing from topic bullet:\n%s", got)
	}
	if !regexp.MustCompile(`- \d{4}-\d{2}-\d{2}: gateway honors`).MatchString(got) {
		t.Fatalf("bullet missing date prefix:\n%s", got)
	}
}

package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/underworld14/pine/internal/memory"
)

func TestWriteMemoryDestUnknownKind(t *testing.T) {
	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	err := writeMemoryDest(cmd, t.TempDir(), "bogus", "x", memory.AppendOpts{Text: "hi"}, false)
	if err == nil {
		t.Fatal("expected unknown kind")
	}
}

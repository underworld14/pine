package cli

import (
	"os"
	"testing"

	"github.com/underworld14/pine/internal/memory"
)

// TestMain pins PINE_HOME to a throwaway dir for the whole package, so a test
// that forgets its own t.Setenv can never write to the developer's real
// ~/.pine. Tests that touch global content must still set their own
// t.Setenv(memory.EnvHome, t.TempDir()): this dir is shared package-wide, so
// without that a stray `learn -g` would leak into a later assertion.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "pine-home-")
	if err != nil {
		panic(err)
	}
	os.Setenv(memory.EnvHome, dir)
	code := m.Run()
	// Must precede os.Exit — a defer would never run.
	os.RemoveAll(dir)
	os.Exit(code)
}

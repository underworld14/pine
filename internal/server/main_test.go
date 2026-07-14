package server

import (
	"os"
	"testing"

	"github.com/underworld14/pine/internal/memory"
)

// TestMain pins PINE_HOME to a throwaway dir for the whole package. The context
// and prompt handlers render through contextgen, which reads the machine-wide
// memory store — without this, server test output would be a function of
// whatever happens to be in the developer's real ~/.pine.
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

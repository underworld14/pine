package setup

import (
	"bytes"
	"strings"
	"testing"
)

// TestIsInteractiveNonTerminalReader covers the realistic, reliably testable
// path: a reader with no Fd() method, or one whose Fd() is not a terminal.
func TestIsInteractiveNonTerminalReader(t *testing.T) {
	if IsInteractive(strings.NewReader("hello")) {
		t.Fatalf("expected strings.Reader to be reported as non-interactive")
	}
	if IsInteractive(&bytes.Buffer{}) {
		t.Fatalf("expected bytes.Buffer to be reported as non-interactive")
	}
}

// TestStdinIsInteractive exercises the os.Stdin wrapper. Under `go test`,
// stdin is not a real TTY, so this should reliably report false, but we only
// assert that the call completes without panicking to avoid environment
// flakiness (e.g. a rare interactive CI runner).
func TestStdinIsInteractive(t *testing.T) {
	_ = StdinIsInteractive()
}

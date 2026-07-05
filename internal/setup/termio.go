package setup

import (
	"io"
	"os"

	"golang.org/x/term"
)

// IsInteractive reports whether r is a terminal (wizard can prompt).
func IsInteractive(r io.Reader) bool {
	f, ok := r.(interface{ Fd() uintptr })
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// StdinIsInteractive is a convenience wrapper for os.Stdin.
func StdinIsInteractive() bool {
	return IsInteractive(os.Stdin)
}

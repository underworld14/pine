package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// EnvHome relocates the machine-wide memory store. Unset/blank → ~/.pine.
	EnvHome = "PINE_HOME"
	// DirGlobal is the machine-wide store's directory name under the user's home.
	DirGlobal = ".pine"
	// ContextGlobalCap is the max bytes of the global MEMORY.md injected into
	// pine context. Deliberately smaller than ContextMEMORYCap: personal
	// preferences are a constitution, not a knowledge base.
	ContextGlobalCap = 2048
)

// DefaultGlobalMEMORY is the seed written to ~/.pine/MEMORY.md on first use.
// It keeps the same H2 skeleton as DefaultMEMORY so AppendMEMORY's insertion
// into ## Log works against either store unchanged.
const DefaultGlobalMEMORY = `# Personal memory

Stable preferences, conventions, and rules that apply to you in every repository.
Agents: prefer appending here (or a topic under memory/) over repeating yourself.
Anything specific to one repository belongs in that repo's .pine/MEMORY.md instead.
If anything here conflicts with a project's memory, the project wins.

## Preferences

## Conventions

## Gotchas

## Log
`

// GlobalDir resolves the machine-wide Pine home: $PINE_HOME when set, else
// <home>/.pine (%USERPROFILE%\.pine on Windows). It creates nothing.
func GlobalDir() (string, error) {
	// A home-lookup failure becomes an empty string so resolveGlobalDir owns
	// the error text: one message, one place.
	home, _ := os.UserHomeDir()
	return resolveGlobalDir(os.Getenv(EnvHome), home)
}

// resolveGlobalDir is the whole resolution rule, as a pure function: no env,
// no disk, no home lookup — so it is exhaustively testable.
func resolveGlobalDir(pineHome, userHome string) (string, error) {
	if v := strings.TrimSpace(pineHome); v != "" {
		// Reject rather than Abs: filepath.Abs resolves against the working
		// directory, so a relative PINE_HOME would silently give a different
		// store per directory — the opposite of "machine-wide" — and scatter
		// stray dirs inside whatever repo the user happens to be standing in.
		if !filepath.IsAbs(v) {
			return "", fmt.Errorf("%s must be an absolute path, got %q", EnvHome, v)
		}
		return filepath.Clean(v), nil
	}
	if strings.TrimSpace(userHome) == "" {
		return "", fmt.Errorf("cannot resolve a home directory for global memory; set %s", EnvHome)
	}
	return filepath.Join(userHome, DirGlobal), nil
}

// GlobalLabel renders dir for humans: "~/.pine" at the default location, else
// dir itself — under $PINE_HOME a tilde path would name a file that does not exist.
func GlobalLabel(dir string) string {
	if home, err := os.UserHomeDir(); err == nil && dir == filepath.Join(home, DirGlobal) {
		return "~/" + DirGlobal
	}
	return filepath.ToSlash(dir)
}

// EnsureGlobalLayout resolves the machine-wide store and creates it with the
// personal seed on first use.
//
// This is the only function that may create ~/.pine. Every read path leaves a
// missing global store missing: pine context must not conjure a store the user
// never opted into.
func EnsureGlobalLayout() (string, error) {
	dir, err := GlobalDir()
	if err != nil {
		return "", err
	}
	if err := ensureLayout(dir, DefaultGlobalMEMORY); err != nil {
		return "", err
	}
	return dir, nil
}

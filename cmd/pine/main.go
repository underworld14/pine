// Command pine is a git-native, local-first workspace for AI-assisted
// development: markdown tickets, a kanban web UI, attachments, search, and
// AI-context generation, all stored as files in a .pine/ directory.
package main

import "github.com/underworld14/pine/internal/cli"

// version is overridden at build time via -ldflags.
var version = "0.1.0-dev"

func main() {
	cli.Execute(version)
}

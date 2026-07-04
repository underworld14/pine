//go:build !embedassets

// Package web provides the embedded SvelteKit build. By default (no build tag)
// no assets are embedded, so `go build` and `go test` work without first
// building the frontend. Building with -tags embedassets (via `make build`)
// embeds web/build produced by the SvelteKit adapter-static output.
package web

import "io/fs"

// Assets is the embedded frontend filesystem, or nil when built without assets.
var Assets fs.FS

// HasAssets reports whether a real frontend build is embedded.
func HasAssets() bool { return false }

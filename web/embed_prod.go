//go:build embedassets

package web

import (
	"embed"
	"io/fs"
)

//go:embed all:build
var buildFS embed.FS

// Assets is the embedded SvelteKit build rooted at web/build.
var Assets fs.FS = mustSub()

func mustSub() fs.FS {
	sub, err := fs.Sub(buildFS, "build")
	if err != nil {
		panic(err)
	}
	return sub
}

// HasAssets reports whether a real frontend build is embedded.
func HasAssets() bool { return true }

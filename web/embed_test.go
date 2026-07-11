package web

import "testing"

// TestHasAssetsDefaultBuild confirms the default (no embedassets build tag)
// contract documented in embed.go: no frontend build is embedded, so `go
// build`/`go test` work without first building the SvelteKit UI.
func TestHasAssetsDefaultBuild(t *testing.T) {
	if HasAssets() {
		t.Error("HasAssets() should be false in the default (non-embedassets) build")
	}
	if Assets != nil {
		t.Error("Assets should be nil in the default (non-embedassets) build")
	}
}

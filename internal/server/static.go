package server

import (
	"bytes"
	"io"
	"io/fs"
	"net/http"
	"strings"

	"github.com/izzadev/pine/web"
)

// placeholderHTML is shown when the binary was built without an embedded
// frontend (plain `go build`). Real builds use `make build` (-tags embedassets).
const placeholderHTML = `<!doctype html>
<meta charset="utf-8">
<title>Pine</title>
<style>
  body{font:15px/1.6 system-ui,sans-serif;background:#0e1210;color:#e6eae7;
       display:grid;place-items:center;min-height:100vh;margin:0}
  .card{max-width:32rem;padding:2rem;text-align:center}
  code{background:#151a17;padding:.15em .4em;border-radius:4px;color:#34d399}
  a{color:#34d399}
</style>
<div class="card">
  <h1>🌲 Pine is running</h1>
  <p>The API is live at <code>/api</code>, but the web UI has not been built into
     this binary.</p>
  <p>Build it with <code>make build</code>, or run the dev server with
     <code>pine serve --dev</code> alongside <code>npm run dev</code> in <code>web/</code>.</p>
</div>
`

// handleStatic serves the embedded SPA, falling back to index.html for client
// routes. Without embedded assets it serves a helpful placeholder.
func (srv *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if !web.HasAssets() {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, placeholderHTML)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	if serveAsset(w, r, path) {
		return
	}
	// SPA fallback for client-side routes.
	serveAsset(w, r, "index.html")
}

func serveAsset(w http.ResponseWriter, r *http.Request, path string) bool {
	data, err := fs.ReadFile(web.Assets, path)
	if err != nil {
		return false
	}
	info, err := fs.Stat(web.Assets, path)
	if err != nil || info.IsDir() {
		return false
	}
	if strings.HasPrefix(path, "_app/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeContent(w, r, path, info.ModTime(), bytes.NewReader(data))
	return true
}

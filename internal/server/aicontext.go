package server

import (
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/underworld14/pine/internal/contextgen"
)

func (srv *Server) handleContext(w http.ResponseWriter, r *http.Request) {
	md := contextgen.Context(srv.store, srv.gitSnapshot(), time.Now())
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Write([]byte(md))
}

func (srv *Server) handlePrompt(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tmpl := ""
	if data, err := os.ReadFile(filepath.Join(srv.store.Root(), "prompts", "fix.md")); err == nil {
		tmpl = string(data)
	}
	md, err := contextgen.Prompt(srv.store, srv.gitSnapshot(), id, tmpl)
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Write([]byte(md))
}

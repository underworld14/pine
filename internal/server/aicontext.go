package server

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/underworld14/pine/internal/contextgen"
	"github.com/underworld14/pine/internal/gitx"
)

func (srv *Server) handleContext(w http.ResponseWriter, r *http.Request) {
	st := srv.storeOf(r)
	git := srv.gitSnapshot()
	if !srv.isActiveStore(st) {
		ctx, cancel := context.WithTimeout(r.Context(), gitTimeout)
		defer cancel()
		git = gitx.New(filepath.Dir(st.Root())).Snapshot(ctx, gitCommitLimit)
	}
	md := contextgen.Context(st, git, time.Now())
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write([]byte(md))
}

func (srv *Server) handlePrompt(w http.ResponseWriter, r *http.Request) {
	st := srv.storeOf(r)
	id := chi.URLParam(r, "id")
	tmpl := ""
	if data, err := os.ReadFile(filepath.Join(st.Root(), "prompts", "fix.md")); err == nil {
		tmpl = string(data)
	}
	git := srv.gitSnapshot()
	if !srv.isActiveStore(st) {
		ctx, cancel := context.WithTimeout(r.Context(), gitTimeout)
		defer cancel()
		git = gitx.New(filepath.Dir(st.Root())).Snapshot(ctx, gitCommitLimit)
	}
	md, err := contextgen.Prompt(st, git, id, tmpl)
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write([]byte(md))
}

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
	"github.com/underworld14/pine/internal/store"
)

func (srv *Server) handleContext(w http.ResponseWriter, r *http.Request) {
	st := srv.storeOf(r)
	git := srv.gitSnapshot()
	if !srv.isActiveStore(st) {
		ctx, cancel := context.WithTimeout(r.Context(), gitTimeout)
		defer cancel()
		git = gitx.New(filepath.Dir(st.Root())).Snapshot(ctx, gitCommitLimit)
	}
	md := contextgen.ContextCompact(st, git, time.Now())
	if r.URL.Query().Get("full") != "" {
		md = contextgen.Context(st, git, time.Now())
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write([]byte(md))
}

// promptTemplateName picks the prompts/<name>.md for a ticket: fix for bug and
// epic tickets, implement for every other type, falling back to fix.md when
// the chosen file is absent (repos initialized before implement.md existed).
func promptTemplateName(st *store.Store, id string) string {
	promptsDir := filepath.Join(st.Root(), "prompts")
	name := "implement"
	if t, err := st.Get(id); err == nil {
		if p := t.Prefix(); p == "BUG" || p == "EPIC" {
			name = "fix"
		}
	}
	if _, err := os.Stat(filepath.Join(promptsDir, name+".md")); err == nil {
		return name
	}
	return "fix"
}

func (srv *Server) handlePrompt(w http.ResponseWriter, r *http.Request) {
	st := srv.storeOf(r)
	id := chi.URLParam(r, "id")
	tmpl := ""
	if name := promptTemplateName(st, id); name != "" {
		if data, err := os.ReadFile(filepath.Join(st.Root(), "prompts", name+".md")); err == nil {
			tmpl = string(data)
		}
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

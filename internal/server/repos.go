package server

import (
	"net/http"
	"path/filepath"

	"github.com/go-chi/chi/v5"
)

func (srv *Server) handleListRepos(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"active": srv.registry.ActiveAlias(),
		"repos":  srv.registry.Entries(),
	})
}

func (srv *Server) handleActivateRepo(w http.ResponseWriter, r *http.Request) {
	alias := chi.URLParam(r, "alias")
	if err := srv.SetActiveRepo(alias); err != nil {
		writeErr(w, notFound(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"active": alias,
		"repos":  srv.registry.Entries(),
	})
}

func (srv *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	st := srv.activeStore()
	payload := map[string]any{
		"ok":      true,
		"version": srv.version,
		"project": st.Config().Project.Name,
		"repo":    filepath.Dir(st.Root()),
	}
	if srv.registry != nil && srv.registry.IsWorkspace() {
		payload["active"] = srv.registry.ActiveAlias()
		payload["repos"] = srv.registry.Entries()
	}
	writeJSON(w, http.StatusOK, payload)
}

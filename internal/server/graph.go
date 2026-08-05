package server

import (
	"net/http"
)

func (srv *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	st := srv.storeOf(r)
	g := st.LinksGraph()
	writeJSON(w, http.StatusOK, map[string]any{
		"nodes":     g.Nodes,
		"edges":     g.Edges,
		"backlinks": g.Backlinks,
		"dangling":  g.Dangling,
	})
}

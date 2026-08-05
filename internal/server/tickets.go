package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/underworld14/pine/internal/store"
	"github.com/underworld14/pine/internal/ticket"
	"github.com/underworld14/pine/internal/view"
)

func (srv *Server) handleListTickets(w http.ResponseWriter, r *http.Request) {
	st := srv.storeOf(r)
	q := r.URL.Query()
	f := store.Filter{
		Status: q.Get("status"),
		Type:   q.Get("type"),
		Label:  q.Get("label"),
		Parent: q.Get("parent"),
	}
	g := st.Graph()
	ts := st.List(f)
	out := make([]view.Ticket, 0, len(ts))
	for _, t := range ts {
		out = append(out, view.Build(st, g, t, false))
	}
	if srv.isActiveStore(st) {
		out = srv.appendOverlay(out, &f)
	}
	writeJSON(w, http.StatusOK, map[string]any{"tickets": out})
}

// createBody is the POST /api/tickets request. opId is echoed on the resulting
// SSE event (M4) so the originating client can suppress its own echo.
type createBody struct {
	Type     string   `json:"type"`
	Title    string   `json:"title"`
	Priority string   `json:"priority"`
	Labels   []string `json:"labels"`
	Deps     []string `json:"deps"`
	Parent   string   `json:"parent"`
	Links    []string `json:"links"`
	Status   string   `json:"status"`
	Body     string   `json:"body"`
	OpID     string   `json:"opId"`
}

func (srv *Server) handleCreateTicket(w http.ResponseWriter, r *http.Request) {
	st := srv.storeOf(r)
	var b createBody
	if err := decodeJSON(r, &b); err != nil {
		writeErr(w, badRequest(err.Error()))
		return
	}
	t, err := st.Create(store.CreateReq{
		Type:     b.Type,
		Title:    b.Title,
		Priority: b.Priority,
		Labels:   b.Labels,
		Deps:     b.Deps,
		Parent:   b.Parent,
		Links:    b.Links,
		Status:   b.Status,
		Body:     b.Body,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	srv.setETag(w, t.ID)
	if srv.isActiveStore(st) {
		srv.reindex(t.ID)
		srv.kickCrossBranch()
	}
	v := view.Build(st, st.Graph(), t, true)
	alias := srv.registry.AliasOf(st)
	srv.emitRepo(alias, "ticket.created", apiOrigin(b.OpID), map[string]any{"ticket": v})
	writeJSON(w, http.StatusCreated, v)
}

func (srv *Server) handleGetTicket(w http.ResponseWriter, r *http.Request) {
	st := srv.storeOf(r)
	id := chi.URLParam(r, "id")
	t, err := st.Get(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	srv.setETag(w, id)
	writeJSON(w, http.StatusOK, view.Build(st, st.Graph(), t, true))
}

// ticketPatch is the PUT/PATCH body. Nil fields are left unchanged.
type ticketPatch struct {
	Title    *string   `json:"title"`
	Status   *string   `json:"status"`
	Priority *string   `json:"priority"`
	Order    *float64  `json:"order"`
	Labels   *[]string `json:"labels"`
	Deps     *[]string `json:"deps"`
	Parent   *string   `json:"parent"`
	Links    *[]string `json:"links"`
	Body     *string   `json:"body"`
	OpID     string    `json:"opId"`
}

func (srv *Server) handleUpdateTicket(w http.ResponseWriter, r *http.Request) {
	st := srv.storeOf(r)
	id := chi.URLParam(r, "id")
	if srv.isActiveStore(st) {
		if branch, ok := srv.offBranchRef(id); ok {
			writeErr(w, readOnlyBranch(id, branch))
			return
		}
	}
	var p ticketPatch
	if err := decodeJSON(r, &p); err != nil {
		writeErr(w, badRequest(err.Error()))
		return
	}
	// Optimistic concurrency: If-Match is checked atomically with the write inside
	// the store, so a lost update cannot slip between the check and the mutation.
	ifm := strings.Trim(r.Header.Get("If-Match"), `"`)
	updated, err := st.UpdateIfMatch(id, ifm, func(u *ticket.Ticket) error {
		if p.Title != nil {
			u.Title = *p.Title
		}
		if p.Status != nil {
			u.Status = *p.Status
		}
		if p.Priority != nil {
			u.Priority = *p.Priority
		}
		if p.Order != nil {
			u.Order = *p.Order
		}
		if p.Labels != nil {
			u.Labels = *p.Labels
		}
		if p.Deps != nil {
			u.Deps = *p.Deps
		}
		if p.Parent != nil {
			u.Parent = *p.Parent
		}
		if p.Links != nil {
			u.Links = *p.Links
		}
		if p.Body != nil {
			u.Body = *p.Body
		}
		return nil
	})
	if errors.Is(err, store.ErrConflict) {
		t, _ := st.Get(id)
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":   map[string]any{"code": "conflict", "message": "ticket changed on disk"},
			"current": view.Build(st, st.Graph(), t, true),
		})
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	srv.setETagFor(w, st, id)
	if srv.isActiveStore(st) {
		srv.reindex(id)
	}
	v := view.Build(st, st.Graph(), updated, true)
	srv.emitRepo(srv.registry.AliasOf(st), "ticket.updated", apiOrigin(p.OpID), map[string]any{"ticket": v})
	writeJSON(w, http.StatusOK, v)
}

// setChecklistBody is the PATCH /checklist request.
type setChecklistBody struct {
	Index   int    `json:"index"`
	Checked bool   `json:"checked"`
	OpID    string `json:"opId"`
}

func (srv *Server) handleSetChecklist(w http.ResponseWriter, r *http.Request) {
	st := srv.storeOf(r)
	id := chi.URLParam(r, "id")
	if srv.isActiveStore(st) {
		if branch, ok := srv.offBranchRef(id); ok {
			writeErr(w, readOnlyBranch(id, branch))
			return
		}
	}
	var b setChecklistBody
	if err := decodeJSON(r, &b); err != nil {
		writeErr(w, badRequest(err.Error()))
		return
	}
	ifm := strings.Trim(r.Header.Get("If-Match"), `"`)
	updated, err := st.UpdateIfMatch(id, ifm, func(t *ticket.Ticket) error {
		nb, ok := ticket.SetChecklistItem(t.Body, b.Index, b.Checked)
		if !ok {
			return badRequest("checklist index out of range")
		}
		t.Body = nb
		return nil
	})
	if errors.Is(err, store.ErrConflict) {
		cur, _ := st.Get(id)
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":   map[string]any{"code": "conflict", "message": "ticket changed on disk"},
			"current": view.Build(st, st.Graph(), cur, true),
		})
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	srv.setETagFor(w, st, id)
	if srv.isActiveStore(st) {
		srv.reindex(id)
	}
	v := view.Build(st, st.Graph(), updated, true)
	srv.emitRepo(srv.registry.AliasOf(st), "ticket.updated", apiOrigin(b.OpID), map[string]any{"ticket": v})
	writeJSON(w, http.StatusOK, v)
}

func (srv *Server) handleDeleteTicket(w http.ResponseWriter, r *http.Request) {
	st := srv.storeOf(r)
	id := chi.URLParam(r, "id")
	if srv.isActiveStore(st) {
		if branch, ok := srv.offBranchRef(id); ok {
			writeErr(w, readOnlyBranch(id, branch))
			return
		}
	}
	if err := st.Delete(id); err != nil {
		writeErr(w, err)
		return
	}
	if srv.isActiveStore(st) {
		srv.deindex(id)
		srv.kickCrossBranch() // a removed local id may now surface from a branch
	}
	srv.emitRepo(srv.registry.AliasOf(st), "ticket.deleted", apiOrigin(r.URL.Query().Get("opId")), map[string]any{"id": id})
	w.WriteHeader(http.StatusNoContent)
}

// setETagFor stamps the response ETag from the given store's content hash.
func (srv *Server) setETagFor(w http.ResponseWriter, st *store.Store, id string) {
	if h, ok := st.Hash(id); ok {
		w.Header().Set("ETag", `"`+h+`"`)
	}
}

// setETag stamps the ETag from the active store (legacy /api routes).
func (srv *Server) setETag(w http.ResponseWriter, id string) {
	srv.setETagFor(w, srv.activeStore(), id)
}

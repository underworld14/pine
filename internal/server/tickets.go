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
	q := r.URL.Query()
	f := store.Filter{
		Status: q.Get("status"),
		Type:   q.Get("type"),
		Label:  q.Get("label"),
		Parent: q.Get("parent"),
	}
	g := srv.store.Graph()
	ts := srv.store.List(f)
	out := make([]view.Ticket, 0, len(ts))
	for _, t := range ts {
		out = append(out, view.Build(srv.store, g, t, false))
	}
	out = srv.appendOverlay(out, &f)
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
	Status   string   `json:"status"`
	Body     string   `json:"body"`
	OpID     string   `json:"opId"`
}

func (srv *Server) handleCreateTicket(w http.ResponseWriter, r *http.Request) {
	var b createBody
	if err := decodeJSON(r, &b); err != nil {
		writeErr(w, badRequest(err.Error()))
		return
	}
	t, err := srv.store.Create(store.CreateReq{
		Type:     b.Type,
		Title:    b.Title,
		Priority: b.Priority,
		Labels:   b.Labels,
		Deps:     b.Deps,
		Parent:   b.Parent,
		Status:   b.Status,
		Body:     b.Body,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	srv.setETag(w, t.ID)
	srv.reindex(t.ID)
	srv.kickCrossBranch() // a new local id may shadow an off-branch copy
	v := view.Build(srv.store, srv.store.Graph(), t, true)
	srv.emit("ticket.created", apiOrigin(b.OpID), map[string]any{"ticket": v})
	writeJSON(w, http.StatusCreated, v)
}

func (srv *Server) handleGetTicket(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t, err := srv.store.Get(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	srv.setETag(w, id)
	writeJSON(w, http.StatusOK, view.Build(srv.store, srv.store.Graph(), t, true))
}

// ticketPatch is the PUT/PATCH body. Nil fields are left unchanged.
type ticketPatch struct {
	Title    *string   `json:"title"`
	Status   *string   `json:"status"`
	Priority *string   `json:"priority"`
	Labels   *[]string `json:"labels"`
	Deps     *[]string `json:"deps"`
	Parent   *string   `json:"parent"`
	Body     *string   `json:"body"`
	OpID     string    `json:"opId"`
}

func (srv *Server) handleUpdateTicket(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if branch, ok := srv.offBranchRef(id); ok {
		writeErr(w, readOnlyBranch(id, branch))
		return
	}
	var p ticketPatch
	if err := decodeJSON(r, &p); err != nil {
		writeErr(w, badRequest(err.Error()))
		return
	}
	// Optimistic concurrency: If-Match is checked atomically with the write inside
	// the store, so a lost update cannot slip between the check and the mutation.
	ifm := strings.Trim(r.Header.Get("If-Match"), `"`)
	updated, err := srv.store.UpdateIfMatch(id, ifm, func(u *ticket.Ticket) error {
		if p.Title != nil {
			u.Title = *p.Title
		}
		if p.Status != nil {
			u.Status = *p.Status
		}
		if p.Priority != nil {
			u.Priority = *p.Priority
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
		if p.Body != nil {
			u.Body = *p.Body
		}
		return nil
	})
	if errors.Is(err, store.ErrConflict) {
		t, _ := srv.store.Get(id)
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":   map[string]any{"code": "conflict", "message": "ticket changed on disk"},
			"current": view.Build(srv.store, srv.store.Graph(), t, true),
		})
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	srv.setETag(w, id)
	srv.reindex(id)
	v := view.Build(srv.store, srv.store.Graph(), updated, true)
	srv.emit("ticket.updated", apiOrigin(p.OpID), map[string]any{"ticket": v})
	writeJSON(w, http.StatusOK, v)
}

// setChecklistBody is the PATCH /checklist request.
type setChecklistBody struct {
	Index   int    `json:"index"`
	Checked bool   `json:"checked"`
	OpID    string `json:"opId"`
}

func (srv *Server) handleSetChecklist(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if branch, ok := srv.offBranchRef(id); ok {
		writeErr(w, readOnlyBranch(id, branch))
		return
	}
	var b setChecklistBody
	if err := decodeJSON(r, &b); err != nil {
		writeErr(w, badRequest(err.Error()))
		return
	}
	ifm := strings.Trim(r.Header.Get("If-Match"), `"`)
	updated, err := srv.store.UpdateIfMatch(id, ifm, func(t *ticket.Ticket) error {
		nb, ok := ticket.SetChecklistItem(t.Body, b.Index, b.Checked)
		if !ok {
			return badRequest("checklist index out of range")
		}
		t.Body = nb
		return nil
	})
	if errors.Is(err, store.ErrConflict) {
		cur, _ := srv.store.Get(id)
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":   map[string]any{"code": "conflict", "message": "ticket changed on disk"},
			"current": view.Build(srv.store, srv.store.Graph(), cur, true),
		})
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	srv.setETag(w, id)
	srv.reindex(id)
	v := view.Build(srv.store, srv.store.Graph(), updated, true)
	srv.emit("ticket.updated", apiOrigin(b.OpID), map[string]any{"ticket": v})
	writeJSON(w, http.StatusOK, v)
}

func (srv *Server) handleDeleteTicket(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if branch, ok := srv.offBranchRef(id); ok {
		writeErr(w, readOnlyBranch(id, branch))
		return
	}
	if err := srv.store.Delete(id); err != nil {
		writeErr(w, err)
		return
	}
	srv.deindex(id)
	srv.kickCrossBranch() // a removed local id may now surface from a branch
	srv.emit("ticket.deleted", apiOrigin(r.URL.Query().Get("opId")), map[string]any{"id": id})
	w.WriteHeader(http.StatusNoContent)
}

func (srv *Server) setETag(w http.ResponseWriter, id string) {
	if h, ok := srv.store.Hash(id); ok {
		w.Header().Set("ETag", `"`+h+`"`)
	}
}

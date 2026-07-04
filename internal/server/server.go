// Package server exposes the store over an HTTP+JSON API and serves the
// embedded web UI. It binds localhost only and defends the no-auth API with
// Host/Origin checks. Live updates (SSE), search, attachments, and git status
// are layered on in later milestones.
package server

import (
	"net/http"
	"path/filepath"
	"sort"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/izzadev/pine/internal/config"
	"github.com/izzadev/pine/internal/store"
	"github.com/izzadev/pine/internal/view"
)

// Server wires the store into HTTP handlers.
type Server struct {
	store   *store.Store
	version string
	hub     *hub
}

// New constructs a server over the given store.
func New(st *store.Store, version string) *Server {
	return &Server{store: st, version: version, hub: newHub()}
}

// Handler builds the chi router with all routes and middleware.
func (srv *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(securityMiddleware)

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", srv.handleHealth)
		r.Get("/snapshot", srv.handleSnapshot)
		r.Route("/tickets", func(r chi.Router) {
			r.Get("/", srv.handleListTickets)
			r.Post("/", srv.handleCreateTicket)
			r.Get("/{id}", srv.handleGetTicket)
			r.Put("/{id}", srv.handleUpdateTicket)
			r.Patch("/{id}", srv.handleUpdateTicket)
			r.Delete("/{id}", srv.handleDeleteTicket)
			r.Post("/{id}/attachments", srv.handleUploadAttachments)
			r.Delete("/{id}/attachments/{name}", srv.handleDeleteAttachment)
		})
		r.Get("/board", srv.handleBoard)
		r.Get("/config", srv.handleGetConfig)
		r.Put("/config", srv.handlePutConfig)
		r.Get("/events", srv.handleEvents)
	})

	r.Get("/attachments/{id}/{name}", srv.handleServeAttachment)
	r.Get("/*", srv.handleStatic)
	return r
}

func (srv *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"version": srv.version,
		"project": srv.store.Config().Project.Name,
		"repo":    filepath.Dir(srv.store.Root()),
	})
}

func (srv *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"tickets": view.BuildAll(srv.store, true),
		"board":   srv.buildBoard(),
		"config":  srv.store.Config(),
		"git":     nil,
		"seq":     0,
	})
}

func (srv *Server) handleBoard(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, srv.buildBoard())
}

func (srv *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, srv.store.Config())
}

func (srv *Server) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		writeErr(w, badRequest("could not read body"))
		return
	}
	cfg, err := config.Parse(body)
	if err != nil {
		writeErr(w, badRequest(err.Error()))
		return
	}
	if err := srv.store.SaveConfig(cfg); err != nil {
		writeErr(w, unprocessable(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, srv.store.Config())
}

// boardResp is the /api/board and snapshot board shape.
type boardResp struct {
	Columns  []config.Column `json:"columns"`
	Unmapped []string        `json:"unmapped"`
}

// buildBoard returns the columns plus any ticket statuses that match no column
// (rendered as an "Other" tray by the UI).
func (srv *Server) buildBoard() boardResp {
	b := srv.store.Board()
	resp := boardResp{Columns: b.Columns, Unmapped: []string{}}
	set := map[string]bool{}
	for _, t := range srv.store.All() {
		if t.Status != "" && !b.HasStatus(t.Status) {
			set[t.Status] = true
		}
	}
	for s := range set {
		resp.Unmapped = append(resp.Unmapped, s)
	}
	sort.Strings(resp.Unmapped)
	return resp
}

// Package server exposes the store over an HTTP+JSON API and serves the
// embedded web UI. It binds localhost only and defends the no-auth API with
// Host/Origin checks. Live updates (SSE), search, attachments, and git status
// are layered on in later milestones.
package server

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/underworld14/pine/internal/config"
	"github.com/underworld14/pine/internal/gitx"
	"github.com/underworld14/pine/internal/store"
	"github.com/underworld14/pine/internal/view"
)

// Server wires the store into HTTP handlers.
type Server struct {
	store    *store.Store
	storeMu  sync.RWMutex // guards srv.store (swapped by SetActiveRepo); read via activeStore()
	registry *StoreRegistry
	version  string
	hub      *hub
	search   *searchIndex
	searchMu sync.RWMutex // guards srv.search (swapped by initSearch on SetActiveRepo)

	git       gitx.Client
	gitMu     sync.RWMutex
	gitStatus gitx.Status
	// gitPollInterval is how often each repo's git poller snapshots. Defaults to
	// gitPollEvery; tests shorten it to drive a poll deterministically.
	gitPollInterval time.Duration

	// Cross-branch overlay: off-branch tickets computed off the request path.
	crossMu    sync.RWMutex
	crossViews []view.Ticket
	crossIDs   map[string]string // off-branch ticket id -> source branch
	crossHash  string            // change-detection hash of crossViews
	ticketsRel string            // git-anchor-relative tickets dir (".pine/tickets")
	crossKick  chan struct{}     // buffered(1): nudge the poller to refresh
}

// New constructs a server over the given store (single-repo).
func New(st *store.Store, version string) *Server {
	return NewWithRegistry(SingleRegistry(st), version)
}

// NewWithRegistry constructs a server over a multi-repo registry. The active
// store backs legacy /api routes; /api/r/{alias}/… addresses a specific repo.
func NewWithRegistry(reg *StoreRegistry, version string) *Server {
	srv := &Server{registry: reg, store: reg.Active(), version: version, hub: newHub(), gitPollInterval: gitPollEvery}
	srv.initSearch()
	srv.initGit()
	srv.initCrossBranch()
	return srv
}

// Registry returns the store registry (never nil after New/NewWithRegistry).
func (srv *Server) Registry() *StoreRegistry { return srv.registry }

// SetActiveRepo switches the active store used by legacy /api routes.
func (srv *Server) SetActiveRepo(alias string) error {
	if srv.registry == nil {
		return fmt.Errorf("no registry")
	}
	if err := srv.registry.SetActive(alias); err != nil {
		return err
	}
	srv.storeMu.Lock()
	srv.store = srv.registry.Active()
	srv.storeMu.Unlock()
	srv.initSearch()
	srv.initGit()
	srv.initCrossBranch()
	return nil
}

// activeStore returns the active store under the lock, so request handlers and
// pollers can't race with SetActiveRepo swapping srv.store.
func (srv *Server) activeStore() *store.Store {
	srv.storeMu.RLock()
	defer srv.storeMu.RUnlock()
	return srv.store
}

// isActiveStore reports whether st is the active store. Reads the active store
// under the lock to avoid racing SetActiveRepo.
func (srv *Server) isActiveStore(st *store.Store) bool {
	return st == srv.activeStore()
}

// storeOf returns the store for this request: alias from /api/r/{alias}/… when
// present, otherwise the active store.
func (srv *Server) storeOf(r *http.Request) *store.Store {
	if alias := chi.URLParam(r, "alias"); alias != "" {
		if st, ok := srv.registry.Get(alias); ok {
			return st
		}
	}
	return srv.activeStore()
}

// Handler builds the chi router with all routes and middleware.
func (srv *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(securityMiddleware)

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", srv.handleHealth)
		r.Get("/repos", srv.handleListRepos)
		r.Post("/repos/{alias}/activate", srv.handleActivateRepo)
		srv.mountStoreRoutes(r)

	r.Route("/r/{alias}", func(r chi.Router) {
		r.Use(srv.requireAlias)
		srv.mountStoreRoutes(r)
		// Attachments are served from the alias store so /api/r/{alias}/attachments/...
		// resolves the right repo's files (the legacy /attachments/... route
		// serves the active store, which is correct after a repo switch).
		r.Get("/attachments/{id}/{name}", srv.handleServeAttachment)
	})
	})

	r.Get("/attachments/{id}/{name}", srv.handleServeAttachment)
	r.Get("/*", srv.handleStatic)
	return r
}

func (srv *Server) mountStoreRoutes(r chi.Router) {
	r.Get("/snapshot", srv.handleSnapshot)
	r.Route("/tickets", func(r chi.Router) {
		r.Get("/", srv.handleListTickets)
		r.Post("/", srv.handleCreateTicket)
		r.Get("/{id}", srv.handleGetTicket)
		r.Put("/{id}", srv.handleUpdateTicket)
		r.Patch("/{id}", srv.handleUpdateTicket)
		r.Patch("/{id}/checklist", srv.handleSetChecklist)
		r.Delete("/{id}", srv.handleDeleteTicket)
		r.Post("/{id}/attachments", srv.handleUploadAttachments)
		r.Delete("/{id}/attachments/{name}", srv.handleDeleteAttachment)
		r.Get("/{id}/prompt", srv.handlePrompt)
	})
	r.Get("/board", srv.handleBoard)
	r.Get("/config", srv.handleGetConfig)
	r.Put("/config", srv.handlePutConfig)
	r.Get("/search", srv.handleSearch)
	r.Get("/git", srv.handleGit)
	r.Get("/files", srv.handleFiles)
	r.Get("/context", srv.handleContext)
	r.Get("/graph", srv.handleGraph)
	r.Get("/events", srv.handleEvents)
}

func (srv *Server) requireAlias(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		alias := chi.URLParam(r, "alias")
		if _, ok := srv.registry.Get(alias); !ok {
			writeErr(w, notFound("unknown repo alias "+alias))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (srv *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	st := srv.storeOf(r)
	tickets := view.BuildAll(st, true)
	board := buildBoardFor(st, nil)
	git := gitx.Status{}
	if srv.isActiveStore(st) {
		tickets = srv.appendOverlay(tickets, nil)
		board = srv.buildBoard()
		git = srv.gitSnapshot()
	} else {
		ctx, cancel := context.WithTimeout(r.Context(), gitTimeout)
		defer cancel()
		git = gitx.New(filepath.Dir(st.Root())).Snapshot(ctx, gitCommitLimit)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tickets": tickets,
		"board":   board,
		"config":  st.Config(),
		"git":     git,
		"seq":     0,
	})
}

func (srv *Server) handleBoard(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, srv.buildBoard())
}

func (srv *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, srv.storeOf(r).Config())
}

func (srv *Server) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	st := srv.storeOf(r)
	body, err := readBody(r)
	if err != nil {
		writeErr(w, badRequest("could not read body"))
		return
	}
	// Overlay onto the current config so omitting a key does not reset it.
	cfg, err := config.ParseOnto(st.Config(), body)
	if err != nil {
		writeErr(w, badRequest(err.Error()))
		return
	}
	if err := st.SaveConfig(cfg); err != nil {
		writeErr(w, unprocessable(err.Error()))
		return
	}
	if srv.isActiveStore(st) {
		srv.kickCrossBranch() // crossBranch.enabled / idStyle may have changed
	}
	writeJSON(w, http.StatusOK, cfg)
}

// boardResp is the /api/board and snapshot board shape.
type boardResp struct {
	Columns  []config.Column `json:"columns"`
	Unmapped []string        `json:"unmapped"`
}

// buildBoard returns the columns plus any ticket statuses that match no column
// (rendered as an "Other" tray by the UI).
func (srv *Server) buildBoard() boardResp {
	return buildBoardFor(srv.activeStore(), srv.crossSnapshot())
}

func buildBoardFor(st *store.Store, extra []view.Ticket) boardResp {
	b := st.Board()
	resp := boardResp{Columns: b.Columns, Unmapped: []string{}}
	set := map[string]bool{}
	for _, t := range st.All() {
		if t.Status != "" && !b.HasStatus(t.Status) {
			set[t.Status] = true
		}
	}
	for _, v := range extra {
		if v.Status != "" && !b.HasStatus(v.Status) {
			set[v.Status] = true
		}
	}
	for s := range set {
		resp.Unmapped = append(resp.Unmapped, s)
	}
	sort.Strings(resp.Unmapped)
	return resp
}
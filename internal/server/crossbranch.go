package server

import (
	"context"
	"encoding/json"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/underworld14/pine/internal/crossbranch"
	"github.com/underworld14/pine/internal/store"
	"github.com/underworld14/pine/internal/view"
)

const (
	crossPollEvery = 30 * time.Second // branch tips move less often than the working tree
	crossTimeout   = 15 * time.Second // a scan shells many git commands
)

// initCrossBranch resolves the tickets pathspec and computes the initial overlay
// so the very first /api/snapshot already carries off-branch tickets.
func (srv *Server) initCrossBranch() {
	// Git is anchored at the parent of .pine, so the tickets dir relative to that
	// anchor is "<pineDirName>/tickets" (".pine/tickets" for the standard layout).
	// Forward slashes: git pathspecs use them on every platform.
	srv.ticketsRel = path.Join(filepath.Base(srv.store.Root()), "tickets")
	srv.crossKick = make(chan struct{}, 1)

	ctx, cancel := context.WithTimeout(context.Background(), crossTimeout)
	defer cancel()
	srv.refreshCrossBranch(ctx)
}

// refreshCrossBranch recomputes the off-branch overlay and swaps it into the
// cache. It returns whether the overlay changed. The feature is gated here (not
// just in config) so a sequential-ID repo never aggregates even if enabled.
func (srv *Server) refreshCrossBranch(ctx context.Context) (changed bool) {
	cfg := srv.store.Config()

	var offs []crossbranch.Off
	if cfg.CrossBranch.Enabled && cfg.IDStyle == "hash" {
		localIDs := map[string]bool{}
		for _, t := range srv.store.All() {
			localIDs[t.ID] = true
		}
		offs = crossbranch.Compute(ctx, srv.git, srv.gitSnapshot().Branch, localIDs, crossbranch.Options{
			Enabled:     true,
			IDStyle:     cfg.IDStyle,
			ActiveDays:  cfg.CrossBranch.ActiveBranchDays,
			TicketsPath: srv.ticketsRel,
		})
	}

	views := make([]view.Ticket, 0, len(offs))
	ids := make(map[string]string, len(offs))
	for _, o := range offs {
		views = append(views, view.BuildOffBranch(o.Ticket, o.Branch, true))
		ids[o.Ticket.ID] = o.Branch
	}
	h := hashViews(views)

	srv.crossMu.Lock()
	changed = h != srv.crossHash
	srv.crossHash = h
	srv.crossViews = views
	srv.crossIDs = ids
	srv.crossMu.Unlock()
	return changed
}

// startCrossBranchPoller recomputes the overlay off the request path on a ticker
// (to catch commits on other branches, which no fs event can report) and on
// demand via crossKick (config toggle, local create/delete). It broadcasts
// crossbranch.updated only when the overlay actually changes.
func (srv *Server) startCrossBranchPoller(done chan struct{}) {
	go func() {
		ticker := time.NewTicker(crossPollEvery)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
			case <-srv.crossKick:
			}
			ctx, cancel := context.WithTimeout(context.Background(), crossTimeout)
			changed := srv.refreshCrossBranch(ctx)
			cancel()
			if changed {
				srv.emit("crossbranch.updated", fsOrigin(), map[string]any{"tickets": srv.crossSnapshot()})
			}
		}
	}()
}

// kickCrossBranch nudges the poller to refresh soon (non-blocking).
func (srv *Server) kickCrossBranch() {
	if srv.crossKick == nil {
		return
	}
	select {
	case srv.crossKick <- struct{}{}:
	default:
	}
}

// crossSnapshot returns a copy of the current off-branch views.
func (srv *Server) crossSnapshot() []view.Ticket {
	srv.crossMu.RLock()
	defer srv.crossMu.RUnlock()
	out := make([]view.Ticket, len(srv.crossViews))
	copy(out, srv.crossViews)
	return out
}

// offBranchRef reports the branch an off-branch (read-only) ticket lives on.
func (srv *Server) offBranchRef(id string) (string, bool) {
	srv.crossMu.RLock()
	defer srv.crossMu.RUnlock()
	b, ok := srv.crossIDs[id]
	return b, ok
}

// appendOverlay adds off-branch views to a set of local views, skipping any ID
// already present locally (local always wins) and, when f is non-nil, any view
// that fails the same filter the store applied.
func (srv *Server) appendOverlay(local []view.Ticket, f *store.Filter) []view.Ticket {
	cross := srv.crossSnapshot()
	if len(cross) == 0 {
		return local
	}
	seen := make(map[string]bool, len(local))
	for _, v := range local {
		seen[v.ID] = true
	}
	for _, v := range cross {
		if seen[v.ID] {
			continue
		}
		if f != nil && !overlayMatches(*f, v) {
			continue
		}
		local = append(local, v)
	}
	return local
}

// overlayMatches mirrors store.Filter.matches for an already-built view.
func overlayMatches(f store.Filter, v view.Ticket) bool {
	if f.Status != "" && v.Status != f.Status {
		return false
	}
	if f.Type != "" && v.Type != strings.ToUpper(f.Type) {
		return false
	}
	if f.Parent != "" && v.Parent != f.Parent {
		return false
	}
	if f.Label != "" {
		found := false
		for _, l := range v.Labels {
			if l == f.Label {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func hashViews(v []view.Ticket) string {
	b, _ := json.Marshal(v)
	return string(b)
}

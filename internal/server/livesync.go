package server

import (
	"log"

	"github.com/underworld14/pine/internal/store"
	"github.com/underworld14/pine/internal/view"
	"github.com/underworld14/pine/internal/watch"
)

// StartLiveSync begins watching the .pine directory and broadcasting external
// changes over SSE. It returns a stop function. When the watcher cannot start,
// the server still runs (without live updates). In workspace mode every repo
// is watched; events are tagged with the repo alias.
func (srv *Server) StartLiveSync() func() {
	done := make(chan struct{})
	srv.startCrossBranchPoller(done)

	type watched struct {
		alias string
		st    *store.Store
		w     *watch.Watcher
	}
	var watchers []watched

	aliases := srv.registry.All()
	for _, alias := range aliases {
		st, ok := srv.registry.Get(alias)
		if !ok || st == nil {
			continue
		}
		// Poll this repo's git state independently so every repo gets live
		// git.updated (the active repo's poller also keeps srv.gitStatus fresh).
		srv.startRepoGitPoller(done, alias, st)
		w, err := watch.New(st.Root())
		if err != nil {
			log.Printf("pine: file watcher disabled for %s: %v", alias, err)
			continue
		}
		watchers = append(watchers, watched{alias: alias, st: st, w: w})
		go func(alias string, st *store.Store, w *watch.Watcher) {
			for {
				select {
				case <-done:
					return
				case batch, ok := <-w.Events():
					if !ok {
						return
					}
					srv.applyWatchBatchFor(alias, st, batch)
				}
			}
		}(alias, st, w)
	}

	if len(watchers) == 0 {
		return func() { close(done) }
	}

	return func() {
		close(done)
		for _, w := range watchers {
			_ = w.w.Close()
		}
	}
}

// applyWatchBatch reconciles a watcher batch for the active store.
func (srv *Server) applyWatchBatch(batch []watch.Event) {
	srv.applyWatchBatchFor(srv.registry.ActiveAlias(), srv.activeStore(), batch)
}

// applyWatchBatchFor reconciles a watcher batch for a specific repo store.
func (srv *Server) applyWatchBatchFor(alias string, st *store.Store, batch []watch.Event) {
	isActive := srv.isActiveStore(st)
	var updatedIDs []string
	for _, ev := range batch {
		switch ev.Kind {
		case watch.KindTicket:
			ch, err := st.ReloadTicket(ev.Path)
			if err != nil {
				continue
			}
			if ch.Removed {
				if isActive {
					srv.deindex(ch.ID)
					srv.kickCrossBranch()
				}
				srv.emitRepo(alias, "ticket.deleted", fsOrigin(), map[string]any{"id": ch.ID})
				continue
			}
			if ch.Changed {
				if isActive {
					srv.reindex(ch.ID)
				}
				updatedIDs = append(updatedIDs, ch.ID)
			}
		case watch.KindConfig:
			if changed, _ := st.ReloadConfig(); changed {
				if isActive {
					srv.kickCrossBranch()
				}
				srv.emitRepo(alias, "config.updated", fsOrigin(), map[string]any{"config": st.Config()})
			}
		case watch.KindBoard:
			if changed, _ := st.ReloadBoard(); changed {
				board := buildBoardFor(st, nil)
				if isActive {
					board = srv.buildBoard()
				}
				srv.emitRepo(alias, "board.updated", fsOrigin(), map[string]any{"board": board})
			}
		case watch.KindLearning:
			_, _ = st.ReloadLearning(ev.Path)
		case watch.KindMemory:
			// A memory topic or MEMORY.md changed: the unified links graph cache
			// is stale. Drop it so the next /api/graph or pine prompt rebuilds.
			// (No SSE event: the graph view polls/refetches on demand.)
			st.InvalidateLinksGraph()
		}
	}
	if len(updatedIDs) == 0 {
		return
	}
	g := st.Graph()
	for _, id := range updatedIDs {
		t, err := st.Get(id)
		if err != nil {
			continue
		}
		srv.emitRepo(alias, "ticket.updated", fsOrigin(), map[string]any{
			"ticket": view.Build(st, g, t, true),
		})
	}
}

// applyWatchEvent reconciles one external change and broadcasts it. Store reloads
// dedupe by content hash, so the server's own writes (already reflected in the
// cache) produce no duplicate event here — the API handler emits those.
func (srv *Server) applyWatchEvent(ev watch.Event) {
	srv.applyWatchBatch([]watch.Event{ev})
}

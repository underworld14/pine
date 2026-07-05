package server

import (
	"log"

	"github.com/underworld14/pine/internal/view"
	"github.com/underworld14/pine/internal/watch"
)

// StartLiveSync begins watching the .pine directory and broadcasting external
// changes over SSE. It returns a stop function. When the watcher cannot start,
// the server still runs (without live updates).
func (srv *Server) StartLiveSync() func() {
	done := make(chan struct{})
	srv.startGitPoller(done)
	srv.startCrossBranchPoller(done)

	w, err := watch.New(srv.store.Root())
	if err != nil {
		log.Printf("pine: file watcher disabled: %v", err)
		return func() { close(done) }
	}
	go func() {
		for {
			select {
			case <-done:
				return
			case batch, ok := <-w.Events():
				if !ok {
					return
				}
				srv.applyWatchBatch(batch)
			}
		}
	}()
	return func() {
		close(done)
		_ = w.Close()
	}
}

// applyWatchBatch reconciles a watcher batch and broadcasts changes. Ticket
// updates share one dependency-graph build per batch instead of one per event.
func (srv *Server) applyWatchBatch(batch []watch.Event) {
	var updatedIDs []string
	for _, ev := range batch {
		switch ev.Kind {
		case watch.KindTicket:
			ch, err := srv.store.ReloadTicket(ev.Path)
			if err != nil {
				continue
			}
			if ch.Removed {
				srv.deindex(ch.ID)
				srv.kickCrossBranch() // a removed local id may now surface from a branch
				srv.emit("ticket.deleted", fsOrigin(), map[string]any{"id": ch.ID})
				continue
			}
			if ch.Changed {
				srv.reindex(ch.ID)
				updatedIDs = append(updatedIDs, ch.ID)
			}
		case watch.KindConfig:
			if changed, _ := srv.store.ReloadConfig(); changed {
				srv.kickCrossBranch() // crossBranch.enabled / idStyle may have changed
				srv.emit("config.updated", fsOrigin(), map[string]any{"config": srv.store.Config()})
			}
		case watch.KindBoard:
			if changed, _ := srv.store.ReloadBoard(); changed {
				srv.emit("board.updated", fsOrigin(), map[string]any{"board": srv.buildBoard()})
			}
		}
	}
	if len(updatedIDs) == 0 {
		return
	}
	g := srv.store.Graph()
	for _, id := range updatedIDs {
		t, err := srv.store.Get(id)
		if err != nil {
			continue
		}
		srv.emit("ticket.updated", fsOrigin(), map[string]any{
			"ticket": view.Build(srv.store, g, t, true),
		})
	}
}

// applyWatchEvent reconciles one external change and broadcasts it. Store reloads
// dedupe by content hash, so the server's own writes (already reflected in the
// cache) produce no duplicate event here — the API handler emits those.
func (srv *Server) applyWatchEvent(ev watch.Event) {
	srv.applyWatchBatch([]watch.Event{ev})
}

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
				for _, ev := range batch {
					srv.applyWatchEvent(ev)
				}
			}
		}
	}()
	return func() {
		close(done)
		_ = w.Close()
	}
}

// applyWatchEvent reconciles one external change and broadcasts it. Store reloads
// dedupe by content hash, so the server's own writes (already reflected in the
// cache) produce no duplicate event here — the API handler emits those.
func (srv *Server) applyWatchEvent(ev watch.Event) {
	switch ev.Kind {
	case watch.KindTicket:
		ch, err := srv.store.ReloadTicket(ev.Path)
		if err != nil {
			return
		}
		if ch.Removed {
			srv.deindex(ch.ID)
			srv.emit("ticket.deleted", fsOrigin(), map[string]any{"id": ch.ID})
			return
		}
		if ch.Changed {
			t, err := srv.store.Get(ch.ID)
			if err != nil {
				return
			}
			srv.reindex(ch.ID)
			srv.emit("ticket.updated", fsOrigin(), map[string]any{
				"ticket": view.Build(srv.store, srv.store.Graph(), t, true),
			})
		}
	case watch.KindConfig:
		if changed, _ := srv.store.ReloadConfig(); changed {
			srv.emit("config.updated", fsOrigin(), map[string]any{"config": srv.store.Config()})
		}
	case watch.KindBoard:
		if changed, _ := srv.store.ReloadBoard(); changed {
			srv.emit("board.updated", fsOrigin(), map[string]any{"board": srv.buildBoard()})
		}
	}
}

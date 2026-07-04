package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// sseMsg is one server-sent event.
type sseMsg struct {
	id    int64
	event string
	data  []byte
}

// hub fans out events to all connected SSE clients. A slow client whose buffer
// fills is skipped for that event (it resyncs via /api/snapshot on reconnect).
type hub struct {
	mu      sync.Mutex
	clients map[chan sseMsg]struct{}
	seq     int64
}

func newHub() *hub {
	return &hub{clients: map[chan sseMsg]struct{}{}}
}

func (h *hub) subscribe() chan sseMsg {
	ch := make(chan sseMsg, 64)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *hub) unsubscribe(ch chan sseMsg) {
	h.mu.Lock()
	if _, ok := h.clients[ch]; ok {
		delete(h.clients, ch)
		close(ch)
	}
	h.mu.Unlock()
}

// broadcast assigns a sequence number, stamps it into the payload, and delivers
// to every client without blocking.
func (h *hub) broadcast(event string, data map[string]any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.seq++
	data["seq"] = h.seq
	payload, err := json.Marshal(data)
	if err != nil {
		return
	}
	msg := sseMsg{id: h.seq, event: event, data: payload}
	for ch := range h.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}

// handleEvents streams events as text/event-stream. On (re)connect the client is
// expected to refetch /api/snapshot; there is no replay buffer.
func (srv *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := srv.hub.subscribe()
	defer srv.hub.unsubscribe(ch)

	io.WriteString(w, ": connected\n\n")
	flusher.Flush()

	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()
	ctx := r.Context()

	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			io.WriteString(w, ": ping\n\n")
			flusher.Flush()
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", msg.id, msg.event, msg.data)
			flusher.Flush()
		}
	}
}

func fsOrigin() map[string]any { return map[string]any{"source": "fs"} }

func apiOrigin(opID string) map[string]any {
	return map[string]any{"source": "api", "opId": opID}
}

// emit broadcasts an event merging origin with a payload.
func (srv *Server) emit(event string, origin, payload map[string]any) {
	if srv.hub == nil {
		return
	}
	data := map[string]any{"origin": origin}
	for k, v := range payload {
		data[k] = v
	}
	srv.hub.broadcast(event, data)
}

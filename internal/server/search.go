package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/underworld14/pine/internal/search"
	"github.com/underworld14/pine/internal/ticket"
)

// searchIndex is an alias so server code refers to one type.
type searchIndex = search.Index

// initSearch builds the index asynchronously from the current tickets.
func (srv *Server) initSearch() {
	idx, err := search.New()
	if err != nil {
		return
	}
	srv.searchMu.Lock()
	srv.search = idx
	srv.searchMu.Unlock()
	all := srv.activeStore().All()
	docs := make([]search.Doc, 0, len(all))
	for _, t := range all {
		docs = append(docs, docFromTicket(t))
	}
	idx.BuildAsync(docs)
}

// searchIdx returns the current search index under the lock, so SetActiveRepo
// (which swaps srv.search) can't race with request handlers / watchers.
func (srv *Server) searchIdx() *searchIndex {
	srv.searchMu.RLock()
	defer srv.searchMu.RUnlock()
	return srv.search
}

func docFromTicket(t *ticket.Ticket) search.Doc {
	return search.Doc{
		ID:           t.ID,
		Title:        t.Title,
		Body:         t.Body,
		Labels:       t.Labels,
		RelatedFiles: strings.Join(ticket.RelatedFiles(t.Body), " "),
		Status:       t.Status,
		Priority:     t.Priority,
		Type:         t.Prefix(),
		Kind:         search.KindTicket,
	}
}

func (srv *Server) reindex(id string) {
	idx := srv.searchIdx()
	if idx == nil {
		return
	}
	if t, err := srv.activeStore().Get(id); err == nil {
		idx.Upsert(docFromTicket(t))
	}
}

func (srv *Server) deindex(id string) {
	if idx := srv.searchIdx(); idx != nil {
		idx.Delete(id)
	}
}

// searchHit joins a search result with display fields from the store.
type searchHit struct {
	ID        string              `json:"id"`
	Score     float64             `json:"score"`
	Title     string              `json:"title"`
	Status    string              `json:"status"`
	Type      string              `json:"type"`
	Fragments map[string][]string `json:"fragments,omitempty"`
}

func (srv *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	st := srv.storeOf(r)
	q := r.URL.Query()
	resp := map[string]any{"indexing": true, "hits": []searchHit{}}
	// The search index is maintained for the active store only. Alias routes to
	// non-active repos return empty results (indexing: true) rather than
	// returning the active repo's tickets — see C1 in the multi-repo review.
	if !srv.isActiveStore(st) {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	idx := srv.searchIdx()
	if idx == nil {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	resp["indexing"] = !idx.Ready()

	limit, _ := strconv.Atoi(q.Get("limit"))
	hits := idx.Search(q.Get("q"), search.Filter{
		Kind:     search.KindTicket,
		Status:   q.Get("status"),
		Type:     q.Get("type"),
		Priority: q.Get("priority"),
	}, limit)

	out := make([]searchHit, 0, len(hits))
	for _, h := range hits {
		sh := searchHit{ID: h.ID, Score: h.Score, Fragments: h.Fragments}
		if t, err := st.Get(h.ID); err == nil {
			sh.Title = t.Title
			sh.Status = t.Status
			sh.Type = t.Prefix()
		}
		out = append(out, sh)
	}
	resp["hits"] = out
	writeJSON(w, http.StatusOK, resp)
}

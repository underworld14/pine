package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/izzadev/pine/internal/search"
	"github.com/izzadev/pine/internal/ticket"
)

// searchIndex is an alias so server code refers to one type.
type searchIndex = search.Index

// initSearch builds the index asynchronously from the current tickets.
func (srv *Server) initSearch() {
	idx, err := search.New()
	if err != nil {
		return
	}
	srv.search = idx
	all := srv.store.All()
	docs := make([]search.Doc, 0, len(all))
	for _, t := range all {
		docs = append(docs, docFromTicket(t))
	}
	idx.BuildAsync(docs)
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
	}
}

func (srv *Server) reindex(id string) {
	if srv.search == nil {
		return
	}
	if t, err := srv.store.Get(id); err == nil {
		srv.search.Upsert(docFromTicket(t))
	}
}

func (srv *Server) deindex(id string) {
	if srv.search != nil {
		srv.search.Delete(id)
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
	q := r.URL.Query()
	resp := map[string]any{"indexing": true, "hits": []searchHit{}}
	if srv.search == nil {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	resp["indexing"] = !srv.search.Ready()

	limit, _ := strconv.Atoi(q.Get("limit"))
	hits := srv.search.Search(q.Get("q"), search.Filter{
		Status:   q.Get("status"),
		Type:     q.Get("type"),
		Priority: q.Get("priority"),
	}, limit)

	out := make([]searchHit, 0, len(hits))
	for _, h := range hits {
		sh := searchHit{ID: h.ID, Score: h.Score, Fragments: h.Fragments}
		if t, err := srv.store.Get(h.ID); err == nil {
			sh.Title = t.Title
			sh.Status = t.Status
			sh.Type = t.Prefix()
		}
		out = append(out, sh)
	}
	resp["hits"] = out
	writeJSON(w, http.StatusOK, resp)
}

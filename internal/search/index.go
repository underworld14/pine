// Package search maintains an in-memory Bleve index over tickets and
// learnings. Nothing is persisted to .pine (the index is rebuilt at startup),
// and updates are applied incrementally as tickets/learnings change.
package search

import (
	"strings"
	"sync/atomic"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/keyword"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/standard"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/blevesearch/bleve/v2/search/highlight/highlighter/html"
	"github.com/blevesearch/bleve/v2/search/query"
)

// Doc is the indexable projection of a ticket or learning.
type Doc struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Body         string   `json:"body"`
	Labels       []string `json:"labels"`
	RelatedFiles string   `json:"relatedFiles"` // space-joined file paths
	Status       string   `json:"status"`
	Priority     string   `json:"priority"`
	Type         string   `json:"type"`
	Kind         string   `json:"kind"`        // "ticket" | "learning"
	Scope        string   `json:"scope"`       // learning only
	Tags         []string `json:"tags"`        // learning only
	Ticket       string   `json:"ticket"`      // learning only
	SourceAgent  string   `json:"sourceAgent"` // learning only
}

// Kind constants for Doc.Kind / Filter.Kind.
const (
	KindTicket   = "ticket"
	KindLearning = "learning"
)

// Filter narrows results to matching field values (all optional).
type Filter struct {
	Status   string
	Type     string
	Priority string
	Kind     string
	Scope    string
	Tags     []string // all must match (AND)
}

// Hit is one search result.
type Hit struct {
	ID        string              `json:"id"`
	Score     float64             `json:"score"`
	Fragments map[string][]string `json:"fragments,omitempty"`
}

// Index wraps a Bleve memory index plus a readiness flag for async builds.
type Index struct {
	idx   bleve.Index
	ready atomic.Bool
}

// New creates an empty in-memory index with Pine's field mapping.
func New() (*Index, error) {
	idx, err := bleve.NewMemOnly(buildMapping())
	if err != nil {
		return nil, err
	}
	return &Index{idx: idx}, nil
}

// BuildAsync indexes docs in the background and marks the index ready when done.
func (i *Index) BuildAsync(docs []Doc) {
	go func() {
		const batchSize = 200
		batch := i.idx.NewBatch()
		n := 0
		for _, d := range docs {
			_ = batch.Index(d.ID, d)
			n++
			if n%batchSize == 0 {
				_ = i.idx.Batch(batch)
				batch = i.idx.NewBatch()
			}
		}
		if batch.Size() > 0 {
			_ = i.idx.Batch(batch)
		}
		i.ready.Store(true)
	}()
}

// Ready reports whether the initial build has finished.
func (i *Index) Ready() bool { return i.ready.Load() }

// Upsert indexes or re-indexes one document.
func (i *Index) Upsert(d Doc) { _ = i.idx.Index(d.ID, d) }

// Delete removes a document.
func (i *Index) Delete(id string) { _ = i.idx.Delete(id) }

// Close releases the index.
func (i *Index) Close() error { return i.idx.Close() }

// Search runs a ranked query and returns hits with highlighted fragments.
func (i *Index) Search(qstr string, f Filter, limit int) []Hit {
	qstr = strings.TrimSpace(qstr)
	if qstr == "" {
		return nil
	}
	if limit <= 0 {
		limit = 20
	}

	must := []query.Query{userTextQuery(qstr)}
	if f.Status != "" {
		must = append(must, termOn("status", strings.ToLower(f.Status)))
	}
	if f.Type != "" {
		must = append(must, termOn("type", strings.ToUpper(f.Type)))
	}
	if f.Priority != "" {
		must = append(must, termOn("priority", strings.ToLower(f.Priority)))
	}
	if f.Kind != "" {
		must = append(must, termOn("kind", strings.ToLower(f.Kind)))
	}
	if f.Scope != "" {
		must = append(must, termOn("scope", strings.ToLower(f.Scope)))
	}
	for _, tag := range f.Tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag != "" {
			must = append(must, termOn("tags", tag))
		}
	}

	req := bleve.NewSearchRequestOptions(bleve.NewConjunctionQuery(must...), limit, 0, false)
	req.Highlight = bleve.NewHighlightWithStyle(html.Name)
	req.Highlight.Fields = []string{"title", "body"}

	res, err := i.idx.Search(req)
	if err != nil {
		return nil
	}
	out := make([]Hit, 0, len(res.Hits))
	for _, h := range res.Hits {
		hit := Hit{ID: h.ID, Score: h.Score}
		if len(h.Fragments) > 0 {
			hit.Fragments = map[string][]string{}
			for field, frags := range h.Fragments {
				hit.Fragments[field] = frags
			}
		}
		out = append(out, hit)
	}
	return out
}

// userTextQuery is a disjunction across fields with id/title boosted.
func userTextQuery(q string) query.Query {
	// The id field is indexed verbatim (keyword analyzer, no lowercasing), so an
	// id query must match the stored casing: uppercase prefix + lowercase hash
	// suffix. Uppercasing the whole query would break hash-id search.
	idq := normalizeIDCase(q)
	idExact := bleve.NewTermQuery(idq)
	idExact.SetField("id")
	idExact.SetBoost(10)

	idPrefix := bleve.NewPrefixQuery(idq)
	idPrefix.SetField("id")
	idPrefix.SetBoost(8)

	titleMatch := bleve.NewMatchQuery(q)
	titleMatch.SetField("title")
	titleMatch.SetFuzziness(1)
	titleMatch.SetBoost(3)

	titlePrefix := bleve.NewPrefixQuery(strings.ToLower(q))
	titlePrefix.SetField("title")
	titlePrefix.SetBoost(2)

	bodyMatch := bleve.NewMatchQuery(q)
	bodyMatch.SetField("body")
	bodyMatch.SetFuzziness(1)

	labelTerm := bleve.NewTermQuery(strings.ToLower(q))
	labelTerm.SetField("labels")

	tagTerm := bleve.NewTermQuery(strings.ToLower(q))
	tagTerm.SetField("tags")

	filesMatch := bleve.NewMatchQuery(q)
	filesMatch.SetField("relatedFiles")

	return bleve.NewDisjunctionQuery(
		idExact, idPrefix, titleMatch, titlePrefix, bodyMatch, labelTerm, tagTerm, filesMatch,
	)
}

// normalizeIDCase uppercases only the id prefix (before the first '-'), matching
// how ids are stored (uppercase prefix + lowercase hash suffix) so that a query
// like "bug-7f3k2a" matches the indexed "BUG-7f3k2a".
func normalizeIDCase(s string) string {
	if i := strings.IndexByte(s, '-'); i >= 0 {
		return strings.ToUpper(s[:i]) + s[i:]
	}
	return strings.ToUpper(s)
}

func termOn(field, value string) query.Query {
	q := bleve.NewTermQuery(value)
	q.SetField(field)
	return q
}

// buildMapping configures analyzers: standard for title/body/files (with stored
// term vectors for highlighting) and keyword for id/labels/status/priority/type.
func buildMapping() mapping.IndexMapping {
	im := bleve.NewIndexMapping()

	text := func() *mapping.FieldMapping {
		fm := bleve.NewTextFieldMapping()
		fm.Analyzer = standard.Name
		fm.Store = true
		fm.IncludeTermVectors = true
		return fm
	}
	kw := func() *mapping.FieldMapping {
		fm := bleve.NewTextFieldMapping()
		fm.Analyzer = keyword.Name
		return fm
	}

	dm := bleve.NewDocumentMapping()
	dm.AddFieldMappingsAt("id", kw())
	dm.AddFieldMappingsAt("title", text())
	dm.AddFieldMappingsAt("body", text())
	dm.AddFieldMappingsAt("relatedFiles", text())
	dm.AddFieldMappingsAt("labels", kw())
	dm.AddFieldMappingsAt("status", kw())
	dm.AddFieldMappingsAt("priority", kw())
	dm.AddFieldMappingsAt("type", kw())
	dm.AddFieldMappingsAt("kind", kw())
	dm.AddFieldMappingsAt("scope", kw())
	dm.AddFieldMappingsAt("tags", kw())
	dm.AddFieldMappingsAt("ticket", kw())
	dm.AddFieldMappingsAt("sourceAgent", kw())

	im.DefaultMapping = dm
	return im
}

package search

import (
	"testing"

	"github.com/underworld14/pine/internal/learning"
)

func TestSearchLearnings(t *testing.T) {
	idx, err := New()
	if err != nil {
		t.Fatal(err)
	}
	docs := []Doc{
		DocFromLearning(&learning.Learning{
			ID: "LRN-001", Scope: learning.ScopeGlobal, Tags: []string{"db"},
			Body: "\nAlways use the query builder for migrations\n",
		}),
		DocFromLearning(&learning.Learning{
			ID: "LRN-002", Scope: learning.ScopeGlobal, Tags: []string{"ui"},
			Body: "\nPrefer CSS variables over hardcoded colors\n",
		}),
		DocFromLearning(&learning.Learning{
			ID: "LRN-003", Scope: learning.ScopeTicket, Ticket: "BUG-001", Tags: []string{"db"},
			Body: "\nSchema drift caused by raw SQL\n",
		}),
	}
	for _, d := range docs {
		idx.Upsert(d)
	}

	hits := idx.Search("query builder", Filter{Kind: KindLearning}, 10)
	if topID(hits) != "LRN-001" {
		t.Fatalf("expected LRN-001, got %v", hits)
	}

	hits = idx.Search("schema", Filter{Kind: KindLearning, Tags: []string{"db"}}, 10)
	if topID(hits) != "LRN-003" {
		t.Fatalf("tag+query filter: got %v", hits)
	}
	if hasID(hits, "LRN-002") {
		t.Fatalf("ui learning should be excluded: %v", hits)
	}

	hits = idx.Search("query", Filter{Kind: KindLearning, Tags: []string{"db"}}, 10)
	if !hasID(hits, "LRN-001") {
		t.Fatalf("expected LRN-001 for query+db: %v", hits)
	}

	hits = idx.Search("schema", Filter{Kind: KindLearning, Scope: learning.ScopeTicket}, 10)
	if topID(hits) != "LRN-003" {
		t.Fatalf("scope ticket: got %v", hits)
	}
}

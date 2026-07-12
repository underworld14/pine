package contextgen

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/underworld14/pine/internal/memory"
)

func TestFormatMemoryBlockNilStore(t *testing.T) {
	if got := FormatMemoryBlock(nil, nil, nil, 3); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatMemoryBlockIncludesMEMORYAndTopics(t *testing.T) {
	s := scaffold(t)
	pine := s.Root()
	if err := memory.EnsureLayout(pine); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	if err := memory.AppendMEMORY(pine, memory.AppendOpts{
		Text: "Prefer query builder for filters",
		Now:  now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := memory.AppendTopic(pine, "analytics", memory.AppendOpts{
		Text:  "usage icons use text-white",
		Cites: []string{"apps/web/src/modules/analytics/lib/usage.ts"},
		Now:   now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := memory.AppendTopic(pine, "billing", memory.AppendOpts{
		Text: "invoice rounding is banker's",
		Now:  now.Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	block := FormatMemoryBlock(s, []string{"apps/web/src/modules/analytics/page.tsx"}, []string{"analytics"}, 2)
	if !strings.Contains(block, "## Project Memory") {
		t.Fatalf("missing MEMORY section:\n%s", block)
	}
	if !strings.Contains(block, "Prefer query builder") {
		t.Fatalf("missing MEMORY body:\n%s", block)
	}
	if !strings.Contains(block, "## Memory Topics") {
		t.Fatalf("missing topics:\n%s", block)
	}
	if !strings.Contains(block, "memory/analytics.md") {
		t.Fatalf("expected analytics topic ranked:\n%s", block)
	}
}

func TestFormatMemoryBlockRecentWhenNoHints(t *testing.T) {
	s := scaffold(t)
	pine := s.Root()
	_ = memory.EnsureLayout(pine)
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	_ = memory.AppendTopic(pine, "old", memory.AppendOpts{Text: "old tip", Now: now.Add(-2 * time.Hour)})
	_ = memory.AppendTopic(pine, "new", memory.AppendOpts{Text: "new tip", Now: now})
	_ = memory.AppendTopic(pine, "mid", memory.AppendOpts{Text: "mid tip", Now: now.Add(-time.Hour)})

	block := FormatMemoryBlock(s, nil, nil, 2)
	if !strings.Contains(block, "memory/new.md") {
		t.Fatalf("expected newest topic:\n%s", block)
	}
	// limit 2 — oldest should be omitted
	if strings.Contains(block, "memory/old.md") {
		t.Fatalf("old topic should be limited out:\n%s", block)
	}
}

func TestFormatMemoryBlockTruncatesLongTopicBody(t *testing.T) {
	s := scaffold(t)
	pine := s.Root()
	_ = memory.EnsureLayout(pine)
	var lines []string
	for i := 0; i < 20; i++ {
		lines = append(lines, "- line "+strings.Repeat("x", 3))
	}
	path := memory.TopicPath(pine, "long")
	body := "---\ntopic: long\nupdated: 2026-07-12T00:00:00Z\n---\n\n# long\n\n" + strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	block := FormatMemoryBlock(s, nil, []string{"long"}, 1)
	if !strings.Contains(block, "…") {
		t.Fatalf("expected excerpt ellipsis:\n%s", block)
	}
}

func TestRankTopicsFillsFromRecent(t *testing.T) {
	topics := []memory.Topic{
		{Slug: "a", RelPath: "memory/a.md", Title: "a", Body: "alpha stuff", Updated: time.Now().Add(-time.Hour)},
		{Slug: "b", RelPath: "memory/b.md", Title: "b", Body: "beta stuff", Updated: time.Now()},
		{Slug: "c", RelPath: "memory/c.md", Title: "c", Body: "gamma stuff", Updated: time.Now().Add(-2 * time.Hour)},
	}
	got := rankTopics(topics, []string{"no-match-zzz"}, nil, 2)
	if len(got) != 2 {
		t.Fatalf("len=%d %#v", len(got), got)
	}
	// With a nonsensical query, bleve may return nothing; fill uses recent → b then a
	if got[0].Slug != "b" {
		t.Fatalf("expected newest first fill, got %#v", got)
	}
}

func TestRankTopicsEmpty(t *testing.T) {
	if got := rankTopics(nil, nil, nil, 3); got != nil {
		t.Fatalf("%#v", got)
	}
}

func TestFormatMemoryBlockDefaultLimit(t *testing.T) {
	s := scaffold(t)
	pine := s.Root()
	_ = memory.EnsureLayout(pine)
	now := time.Now().UTC()
	for _, slug := range []string{"t1", "t2", "t3", "t4"} {
		_ = memory.AppendTopic(pine, slug, memory.AppendOpts{Text: slug + " tip", Now: now})
		now = now.Add(time.Minute)
	}
	block := FormatMemoryBlock(s, nil, nil, 0) // default 3
	count := strings.Count(block, "### ")
	if count != 3 {
		t.Fatalf("want 3 topics, got %d:\n%s", count, block)
	}
}

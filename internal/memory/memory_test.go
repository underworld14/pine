package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnsureLayoutAndAppendMEMORY(t *testing.T) {
	dir := t.TempDir()
	pine := filepath.Join(dir, ".pine")
	if err := os.MkdirAll(pine, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := EnsureLayout(pine); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(MemoryPath(pine)); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	if err := AppendMEMORY(pine, AppendOpts{
		Text:  "Prefer dark mode in the dashboard",
		Cites: nil,
		Now:   now,
	}); err != nil {
		t.Fatal(err)
	}
	body, err := ReadMEMORY(pine)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "## Log") || !strings.Contains(body, "Prefer dark mode") {
		t.Fatalf("MEMORY missing append:\n%s", body)
	}
	if !strings.Contains(body, "2026-07-12") {
		t.Fatalf("date missing:\n%s", body)
	}
}

func TestAppendTopicCreatesAndAppends(t *testing.T) {
	dir := t.TempDir()
	pine := filepath.Join(dir, ".pine")
	_ = os.MkdirAll(pine, 0o755)
	now := time.Date(2026, 7, 12, 1, 0, 0, 0, time.UTC)
	if err := AppendTopic(pine, "Analytics UI", AppendOpts{
		Text:  "type icons use text-white",
		Cites: []string{"apps/web/src/modules/analytics/lib/x.ts"},
		Now:   now,
	}); err != nil {
		t.Fatal(err)
	}
	topics, err := ListTopics(pine)
	if err != nil || len(topics) != 1 {
		t.Fatalf("topics=%v err=%v", topics, err)
	}
	if topics[0].Slug != "analytics-ui" {
		t.Fatalf("slug=%q", topics[0].Slug)
	}
	if err := AppendTopic(pine, "analytics-ui", AppendOpts{
		Text: "never use bg-muted fallback",
		Now:  now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadTopic(pine, "analytics-ui")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Body, "text-white") || !strings.Contains(got.Body, "bg-muted") {
		t.Fatalf("body:\n%s", got.Body)
	}
	if !strings.Contains(got.Body, "cites: apps/web") {
		t.Fatalf("cites missing:\n%s", got.Body)
	}
}

func TestSuggestPrefersCiteTopic(t *testing.T) {
	dir := t.TempDir()
	pine := filepath.Join(dir, ".pine")
	_ = os.MkdirAll(pine, 0o755)
	_ = AppendTopic(pine, "analytics", AppendOpts{
		Text:  "usage type display icons",
		Cites: []string{"apps/web/src/modules/analytics/lib/usage-type-display.ts"},
		Now:   time.Now().UTC(),
	})
	recs, err := Suggest(pine, SuggestOpts{
		Text:  "AI Usage Logs type icons always use text-white on colored square",
		Cites: []string{"apps/web/src/modules/analytics/lib/usage-type-display.ts"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) == 0 {
		t.Fatal("no recommendations")
	}
	if recs[0].Path != "memory/analytics.md" {
		t.Fatalf("expected analytics topic first, got %#v", recs)
	}
	if !Confident(recs) {
		// cite boost should make this confident enough in most cases
		t.Logf("recs=%#v (confidence optional)", recs)
	}
}

func TestResolveTo(t *testing.T) {
	cases := []struct {
		in, kind, value string
	}{
		{"MEMORY.md", "memory", "MEMORY.md"},
		{".pine/MEMORY.md", "memory", "MEMORY.md"},
		{"memory/analytics.md", "topic", "analytics"},
		{"NEW:usage-logs", "new", "usage-logs"},
		{"analytics", "topic", "analytics"},
	}
	for _, c := range cases {
		k, v, err := ResolveTo(c.in)
		if err != nil || k != c.kind || v != c.value {
			t.Errorf("ResolveTo(%q)=%s,%s,%v want %s,%s", c.in, k, v, err, c.kind, c.value)
		}
	}
}

func TestConfidentThresholds(t *testing.T) {
	if Confident(nil) {
		t.Fatal("empty")
	}
	if Confident([]Recommendation{{Path: "NEW:x", Score: 0.9}}) {
		t.Fatal("must not auto-create")
	}
	if !Confident([]Recommendation{
		{Path: "memory/a.md", Score: 0.8},
		{Path: "MEMORY.md", Score: 0.2},
	}) {
		t.Fatal("should be confident")
	}
	if Confident([]Recommendation{
		{Path: "memory/a.md", Score: 0.5},
		{Path: "MEMORY.md", Score: 0.4},
	}) {
		t.Fatal("below threshold")
	}
}

func TestTruncateForContext(t *testing.T) {
	short := "hello\n"
	if got := TruncateForContext(short, 100); got != short {
		t.Fatalf("short: %q", got)
	}
	long := strings.Repeat("line\n", 1000)
	got := TruncateForContext(long, 80)
	if len(got) >= len(long) {
		t.Fatalf("expected truncation, len=%d", len(got))
	}
	if !strings.Contains(got, "truncated") {
		t.Fatalf("missing notice:\n%s", got)
	}
	got = TruncateForContext(long, 0) // default cap
	if len(got) > ContextMEMORYCap+80 {
		t.Fatalf("default cap too large: %d", len(got))
	}
}

func TestSlugifyEdgeCases(t *testing.T) {
	if got := Slugify("  Hello World!! "); got != "hello-world" {
		t.Fatalf("got %q", got)
	}
	if got := Slugify("@@@"); got != "general" {
		t.Fatalf("empty slug: %q", got)
	}
	long := strings.Repeat("abc-", 20)
	if got := Slugify(long); len(got) > 48 {
		t.Fatalf("len=%d %q", len(got), got)
	}
}

func TestResolveToErrorsAndBareMD(t *testing.T) {
	if _, _, err := ResolveTo("foo/bar/baz.md"); err == nil {
		t.Fatal("expected error for nested non-memory path")
	}
	k, v, err := ResolveTo("analytics.md")
	if err != nil || k != "topic" || v != "analytics" {
		t.Fatalf("got %s %s %v", k, v, err)
	}
	k, v, err = ResolveTo("")
	if err != nil || k != "memory" {
		t.Fatalf("empty: %s %s %v", k, v, err)
	}
}

func TestAppendMEMORYCreatesLogHeading(t *testing.T) {
	dir := t.TempDir()
	pine := filepath.Join(dir, ".pine")
	_ = os.MkdirAll(pine, 0o755)
	path := MemoryPath(pine)
	if err := os.WriteFile(path, []byte("# Project memory\n\n## Preferences\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AppendMEMORY(pine, AppendOpts{Text: "no log yet"}); err != nil {
		t.Fatal(err)
	}
	body, _ := ReadMEMORY(pine)
	if !strings.Contains(body, "## Log") || !strings.Contains(body, "no log yet") {
		t.Fatalf("body:\n%s", body)
	}
	if err := AppendMEMORY(pine, AppendOpts{Text: ""}); err == nil {
		t.Fatal("empty text should fail")
	}
}

func TestAppendTopicMiddleAndEmpty(t *testing.T) {
	dir := t.TempDir()
	pine := filepath.Join(dir, ".pine")
	_ = os.MkdirAll(pine, 0o755)
	if err := AppendTopic(pine, "x", AppendOpts{Text: ""}); err == nil {
		t.Fatal("empty should fail")
	}
	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	if err := AppendTopic(pine, "db", AppendOpts{Text: "first", Now: now}); err != nil {
		t.Fatal(err)
	}
	// append again refreshes frontmatter
	if err := AppendTopic(pine, "db", AppendOpts{Text: "second", Now: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadTopic(pine, "db")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Body, "first") || !strings.Contains(got.Body, "second") {
		t.Fatalf("body:\n%s", got.Body)
	}
}

func TestSuggestNewSlugAndComponent(t *testing.T) {
	dir := t.TempDir()
	pine := filepath.Join(dir, ".pine")
	_ = os.MkdirAll(pine, 0o755)
	_ = EnsureLayout(pine)

	recs, err := Suggest(pine, SuggestOpts{
		Text:      "always prefer atomic renames when writing tickets",
		Component: "internal/store",
	})
	if err != nil {
		t.Fatal(err)
	}
	foundNew := false
	for _, r := range recs {
		if strings.HasPrefix(r.Path, "NEW:") {
			foundNew = true
			if r.Path != "NEW:store" {
				t.Fatalf("expected NEW:store from component, got %q (all=%#v)", r.Path, recs)
			}
		}
	}
	if !foundNew {
		t.Fatalf("expected NEW suggestion, got %#v", recs)
	}

	recs, err = Suggest(pine, SuggestOpts{
		Text:  "widget quirk",
		Cites: []string{"apps/web/src/modules/billing/lib/widget.ts"},
	})
	if err != nil {
		t.Fatal(err)
	}
	foundNew = false
	for _, r := range recs {
		if r.Path == "NEW:billing" || r.Path == "NEW:widget" {
			foundNew = true
		}
	}
	if !foundNew {
		t.Fatalf("expected NEW from cite path, got %#v", recs)
	}

	if _, err := Suggest(pine, SuggestOpts{Text: "  "}); err == nil {
		t.Fatal("empty text should fail")
	}
}

func TestSuggestConfidentMemoryOnly(t *testing.T) {
	dir := t.TempDir()
	pine := filepath.Join(dir, ".pine")
	_ = os.MkdirAll(pine, 0o755)
	_ = EnsureLayout(pine)
	_ = AppendMEMORY(pine, AppendOpts{Text: "prefer dark mode chrome for dashboards"})
	recs, err := Suggest(pine, SuggestOpts{Text: "prefer dark mode chrome for dashboards"})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) == 0 {
		t.Fatal("no recs")
	}
	// MEMORY should be present; confidence may or may not fire
	found := false
	for _, r := range recs {
		if r.Path == FileMEMORY {
			found = true
		}
	}
	if !found {
		t.Fatalf("MEMORY missing: %#v", recs)
	}
}

func TestAppendUnderHeadingBetweenSections(t *testing.T) {
	content := "# T\n\n## Log\n- a\n\n## Other\n- b\n"
	got := appendUnderHeading(content, "## Log", "- c")
	if !strings.Contains(got, "- a\n- c\n\n## Other") {
		t.Fatalf("got:\n%s", got)
	}
	got = appendUnderHeading("# T\n", "## Log", "- x")
	if !strings.Contains(got, "## Log\n- x") {
		t.Fatalf("missing heading path:\n%s", got)
	}
}

func TestListTopicsSkipsDirsAndBad(t *testing.T) {
	dir := t.TempDir()
	pine := filepath.Join(dir, ".pine")
	_ = EnsureLayout(pine)
	_ = os.MkdirAll(filepath.Join(TopicsDir(pine), "subdir"), 0o755)
	_ = os.WriteFile(filepath.Join(TopicsDir(pine), "ok.md"), []byte("# ok\n\n- note\n"), 0o644)
	topics, err := ListTopics(pine)
	if err != nil {
		t.Fatal(err)
	}
	if len(topics) != 1 || topics[0].Slug != "ok" {
		t.Fatalf("%#v", topics)
	}
	missing, err := ListTopics(filepath.Join(dir, "nope"))
	if err != nil || missing != nil {
		t.Fatalf("missing dir: %v %#v", err, missing)
	}
}

func TestCiteBoostAndExtractCites(t *testing.T) {
	if b := citeBoost("analytics", []string{"apps/web/src/modules/analytics/x.ts"}, ""); b <= 0 {
		t.Fatalf("boost=%v", b)
	}
	if b := citeBoost("store", nil, "internal/store"); b <= 0 {
		t.Fatalf("component boost=%v", b)
	}
	got := extractCites("- 2026-01-01: tip (cites: a.go, b/c.ts)\n- plain\n")
	if !strings.Contains(got, "a.go") || !strings.Contains(got, "b/c.ts") {
		t.Fatalf("%q", got)
	}
}

func TestEnsureLayoutWhenMEMORYExists(t *testing.T) {
	dir := t.TempDir()
	pine := filepath.Join(dir, ".pine")
	_ = os.MkdirAll(pine, 0o755)
	path := MemoryPath(pine)
	if err := os.WriteFile(path, []byte("# custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureLayout(pine); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "# custom\n" {
		t.Fatalf("existing MEMORY overwritten: %q", body)
	}
}

func TestReadMEMORYMissing(t *testing.T) {
	dir := t.TempDir()
	pine := filepath.Join(dir, ".pine")
	body, err := ReadMEMORY(pine)
	if err != nil || body != "" {
		t.Fatalf("missing MEMORY: body=%q err=%v", body, err)
	}
}

func TestJoinFrontmatterExtraKeys(t *testing.T) {
	got := joinFrontmatter(map[string]string{
		"updated": "2026-07-12T00:00:00Z",
		"topic":   "analytics",
		"owner":   "team-a",
		"extra":   "yes",
	})
	if !strings.HasPrefix(got, "---\ntopic: analytics\nupdated: 2026-07-12T00:00:00Z\n") {
		t.Fatalf("stable keys first:\n%s", got)
	}
	if !strings.Contains(got, "extra: yes\n") || !strings.Contains(got, "owner: team-a\n") {
		t.Fatalf("extra keys missing:\n%s", got)
	}
	// Extra keys sorted alphabetically.
	if i, j := strings.Index(got, "extra:"), strings.Index(got, "owner:"); i < 0 || j < 0 || i > j {
		t.Fatalf("extra keys not sorted:\n%s", got)
	}
}

func TestSplitFrontmatterBadYAML(t *testing.T) {
	fm, rest, ok := splitFrontmatter("---\n: bad\n---\nbody\n")
	if ok || fm != nil || rest != "---\n: bad\n---\nbody\n" {
		t.Fatalf("bad yaml should fail: ok=%v fm=%v rest=%q", ok, fm, rest)
	}
	fm, rest, ok = splitFrontmatter("---\nno end\n")
	if ok || fm != nil {
		t.Fatalf("missing end: ok=%v", ok)
	}
	_ = rest
}

func TestSuggestNewSlugTextOnlyStopwords(t *testing.T) {
	slug := suggestNewSlug("the and for with always prefer use dark mode chrome", nil, "")
	if slug != "dark-mode-chrome" {
		t.Fatalf("got %q", slug)
	}
	if got := suggestNewSlug("the and for", nil, ""); got != "general" {
		t.Fatalf("all stopwords: %q", got)
	}
}

func TestConfidentSingleRecAboveThreshold(t *testing.T) {
	if !Confident([]Recommendation{{Path: "MEMORY.md", Score: AutoMinScore}}) {
		t.Fatal("single rec at threshold should be confident")
	}
	if Confident([]Recommendation{{Path: "MEMORY.md", Score: AutoMinScore - 0.01}}) {
		t.Fatal("single rec below threshold should not be confident")
	}
}

func TestSuggestNewSlugCiteFallsThroughDirs(t *testing.T) {
	// Only skip dirs → use file base.
	if got := suggestNewSlug("x", []string{"src/lib/internal/app/widgets.ts"}, ""); got != "widgets" {
		t.Fatalf("got %q", got)
	}
	if got := suggestNewSlug("a b", nil, ""); got != "general" { // words < 3 chars
		t.Fatalf("short words: %q", got)
	}
}

func TestCiteBoostCapAndEmptyParts(t *testing.T) {
	cites := []string{
		"modules/analytics/analytics/analytics/analytics.ts",
		"/",
		"analytics/foo.go",
	}
	if b := citeBoost("analytics", cites, "modules/analytics"); b != 0.5 {
		t.Fatalf("expected cap 0.5, got %v", b)
	}
}

func TestAppendTopicNoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	pine := filepath.Join(dir, ".pine")
	_ = EnsureLayout(pine)
	path := TopicPath(pine, "nl")
	// No trailing newline; AppendTopic should add one before the bullet.
	body := "---\ntopic: nl\nupdated: 2026-07-12T00:00:00Z\n---\n\n# nl\n\n- old"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	if err := AppendTopic(pine, "nl", AppendOpts{Text: "new tip", Now: now}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "- old\n- 2026-07-12: new tip\n") {
		t.Fatalf("body:\n%s", got)
	}
}

func TestReadTopicFrontmatterUpdated(t *testing.T) {
	dir := t.TempDir()
	pine := filepath.Join(dir, ".pine")
	_ = EnsureLayout(pine)
	path := TopicPath(pine, "ts")
	body := "---\ntopic: ts\nupdated: \"2025-01-02T03:04:05Z\"\n---\n\n# Title Here\n\nnote\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadTopic(pine, "ts")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Title Here" {
		t.Fatalf("title=%q", got.Title)
	}
	want := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	if !got.Updated.Equal(want) {
		t.Fatalf("updated=%v want %v", got.Updated, want)
	}
}

func TestListTopicsSkipsUnreadable(t *testing.T) {
	dir := t.TempDir()
	pine := filepath.Join(dir, ".pine")
	_ = EnsureLayout(pine)
	bad := filepath.Join(TopicsDir(pine), "bad.md")
	if err := os.WriteFile(bad, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Make unreadable so ReadTopic fails and ListTopics skips it.
	if err := os.Chmod(bad, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(bad, 0o644) })
	_ = os.WriteFile(filepath.Join(TopicsDir(pine), "good.md"), []byte("# good\n"), 0o644)
	topics, err := ListTopics(pine)
	if err != nil {
		t.Fatal(err)
	}
	if len(topics) != 1 || topics[0].Slug != "good" {
		t.Fatalf("%#v", topics)
	}
}

func TestSuggestSkipsExistingNewSlug(t *testing.T) {
	dir := t.TempDir()
	pine := filepath.Join(dir, ".pine")
	_ = EnsureLayout(pine)
	_ = AppendTopic(pine, "dark-mode-chrome", AppendOpts{Text: "existing topic about unrelated stuff"})
	recs, err := Suggest(pine, SuggestOpts{
		Text: "the and for with always prefer use dark mode chrome",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range recs {
		if r.Path == "NEW:dark-mode-chrome" {
			t.Fatalf("should not suggest existing slug: %#v", recs)
		}
	}
}

func TestSuggestSoftCiteWithoutBodyHit(t *testing.T) {
	dir := t.TempDir()
	pine := filepath.Join(dir, ".pine")
	_ = EnsureLayout(pine)
	// Topic body has no overlap with the insight text; cite path still soft-matches.
	_ = AppendTopic(pine, "billing", AppendOpts{
		Text: "invoice rounding is banker",
		Now:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	recs, err := Suggest(pine, SuggestOpts{
		Text:  "completely unrelated xyzzy phrase about widgets",
		Cites: []string{"apps/web/src/modules/billing/lib/invoice.ts"},
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range recs {
		if r.Path == "memory/billing.md" && strings.Contains(r.Reason, "cite") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected soft cite recommendation: %#v", recs)
	}
}

func TestSuggestTruncatesToEight(t *testing.T) {
	dir := t.TempDir()
	pine := filepath.Join(dir, ".pine")
	_ = EnsureLayout(pine)
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 10; i++ {
		slug := "topic-" + string(rune('a'+i))
		_ = AppendTopic(pine, slug, AppendOpts{
			Text: "shared widget styling tip number " + slug,
			Now:  now,
		})
	}
	recs, err := Suggest(pine, SuggestOpts{Text: "shared widget styling tip"})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) > 8 {
		t.Fatalf("expected <=8, got %d", len(recs))
	}
}

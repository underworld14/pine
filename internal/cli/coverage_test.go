package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/underworld14/pine/internal/view"
)

// --- pure helpers: truncate / humanBytes / depSummary ---

func TestTruncateShortUnchanged(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("got %q", got)
	}
}

func TestTruncateLongGetsEllipsis(t *testing.T) {
	s := "this is a long title that needs truncating"
	got := truncate(s, 10)
	want := s[:9] + "…"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestHumanBytesBytes(t *testing.T) {
	if got := humanBytes(512); got != "512 B" {
		t.Errorf("got %q", got)
	}
}

func TestHumanBytesKB(t *testing.T) {
	if got := humanBytes(2048); got != "2.0 KB" {
		t.Errorf("got %q", got)
	}
}

func TestHumanBytesMB(t *testing.T) {
	if got := humanBytes(5 * 1024 * 1024); got != "5.0 MB" {
		t.Errorf("got %q", got)
	}
}

func TestHumanBytesGB(t *testing.T) {
	if got := humanBytes(3 * 1024 * 1024 * 1024); got != "3.0 GB" {
		t.Errorf("got %q", got)
	}
}

func TestDepSummaryBranches(t *testing.T) {
	cases := []struct {
		name string
		v    view.Ticket
		want string
	}{
		{"cycle", view.Ticket{InCycle: true}, "🔒 cycle"},
		{"unmet", view.Ticket{Unmet: []string{"BUG-001"}}, "🔒 1 unmet"},
		{"dangling", view.Ticket{Dangling: []string{"BUG-999"}}, "⚠ 1 dangling"},
		{"ready", view.Ticket{Deps: []string{"BUG-001"}}, "ready"},
		{"none", view.Ticket{}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := depSummary(c.v); got != c.want {
				t.Errorf("got %q want %q", got, c.want)
			}
		})
	}
}

// --- server health check ---

func TestServerHealthyTrue(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/health" {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer ts.Close()
	if !serverHealthy(ts.URL) {
		t.Error("expected healthy")
	}
}

func TestServerHealthyFalseUnreachable(t *testing.T) {
	if serverHealthy("http://127.0.0.1:1") {
		t.Error("expected unhealthy for unreachable address")
	}
}

func TestServerHealthyFalseNon200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	if serverHealthy(ts.URL) {
		t.Error("expected unhealthy for 500 response")
	}
}

// --- ticket list/show/ready text output ---

func TestListShowReadyTextOutput(t *testing.T) {
	dir := initRepo(t)

	readyEmpty, err := run(t, dir, "ready")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readyEmpty, "Nothing ready") {
		t.Fatalf("expected nothing-ready message on empty workspace:\n%s", readyEmpty)
	}

	run(t, dir, "create", "--type", "epic", "--title", "Auth epic")                                                     // EPIC-001
	run(t, dir, "create", "--type", "bug", "--title", "Login broken", "-p", "high", "-l", "ui", "--parent", "EPIC-001") // BUG-001
	run(t, dir, "create", "--type", "feature", "--title", "dep target")                                                 // FEAT-001
	run(t, dir, "dep", "add", "BUG-001", "FEAT-001")

	listOut, err := run(t, dir, "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listOut, "BUG-001") || !strings.Contains(listOut, "STATUS") {
		t.Fatalf("list text output missing table:\n%s", listOut)
	}

	showOut, err := run(t, dir, "show", "BUG-001")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(showOut, "deps:") || !strings.Contains(showOut, "BLOCKED") {
		t.Fatalf("show text output missing blocked deps:\n%s", showOut)
	}

	epicOut, err := run(t, dir, "show", "EPIC-001")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(epicOut, "children (0/1 done)") {
		t.Fatalf("show epic missing children progress:\n%s", epicOut)
	}

	readyNonEmpty, err := run(t, dir, "ready")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readyNonEmpty, "FEAT-001") {
		t.Fatalf("expected FEAT-001 to be ready (BUG-001 stays blocked on it):\n%s", readyNonEmpty)
	}
	if strings.Contains(readyNonEmpty, "BUG-001") {
		t.Fatalf("BUG-001 should still be blocked:\n%s", readyNonEmpty)
	}
}

func TestShowDegradedTicket(t *testing.T) {
	dir := initRepo(t)
	os.WriteFile(filepath.Join(dir, ".pine", "tickets", "BUG-001.md"), []byte("no frontmatter here\n"), 0o644)
	out, err := run(t, dir, "show", "BUG-001")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "degraded") {
		t.Fatalf("expected degraded note:\n%s", out)
	}
}

func TestListEmptyWorkspace(t *testing.T) {
	dir := initRepo(t)
	out, err := run(t, dir, "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "No tickets.") {
		t.Fatalf("expected no-tickets message:\n%s", out)
	}
}

// --- dep tree / rm ---

func TestDepTreeAndRm(t *testing.T) {
	dir := initRepo(t)
	run(t, dir, "create", "--type", "bug", "--title", "root")  // BUG-001
	run(t, dir, "create", "--type", "bug", "--title", "child") // BUG-002
	run(t, dir, "dep", "add", "BUG-001", "BUG-002")
	run(t, dir, "dep", "add", "BUG-001", "GHOST-999") // dep add does not validate existence

	treeOut, err := run(t, dir, "dep", "tree", "BUG-001")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(treeOut, "BUG-002") || !strings.Contains(treeOut, "GHOST-999 (missing)") {
		t.Fatalf("dep tree missing expected nodes:\n%s", treeOut)
	}

	rmOut, err := run(t, dir, "dep", "rm", "BUG-001", "GHOST-999")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rmOut, "BUG-002") || strings.Contains(rmOut, "GHOST-999") {
		t.Fatalf("dep rm output unexpected:\n%s", rmOut)
	}

	rmAllOut, err := run(t, dir, "dep", "rm", "BUG-001", "BUG-002")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rmAllOut, "no dependencies") {
		t.Fatalf("expected no-deps message:\n%s", rmAllOut)
	}
}

func TestDepTreeCycleOnDisk(t *testing.T) {
	// dep add refuses cycles, so a real on-disk cycle (to exercise
	// printDepTree's cycle branch) must be hand-planted, mirroring
	// internal/doctor's writeDeps test pattern.
	dir := initRepo(t)
	run(t, dir, "create", "--type", "bug", "--title", "a") // BUG-001
	run(t, dir, "create", "--type", "bug", "--title", "b") // BUG-002
	writeCLIDeps(t, dir, "BUG-001", []string{"BUG-002"})
	writeCLIDeps(t, dir, "BUG-002", []string{"BUG-001"})

	out, err := run(t, dir, "dep", "tree", "BUG-001")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "cycle") {
		t.Fatalf("expected cycle marker in tree output:\n%s", out)
	}
}

func writeCLIDeps(t *testing.T, dir, id string, deps []string) {
	t.Helper()
	path := filepath.Join(dir, ".pine", "tickets", id+".md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	depBlock := "deps:\n"
	for _, d := range deps {
		depBlock += "  - " + d + "\n"
	}
	s := string(raw)
	idx := strings.Index(s, "\n---\n")
	if idx < 0 {
		t.Fatalf("no frontmatter in %s", id)
	}
	updated := s[:idx+1] + depBlock + s[idx+1:]
	os.WriteFile(path, []byte(updated), 0o644)
}

// --- export / context / prompt ---

func TestExportMarkdownAndJSON(t *testing.T) {
	dir := initRepo(t)
	run(t, dir, "create", "--type", "bug", "--title", "exported bug")

	md, err := run(t, dir, "export")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(md, "exported bug") {
		t.Fatalf("markdown export missing ticket:\n%s", md)
	}

	js, err := run(t, dir, "export", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(js, "\"exported bug\"") {
		t.Fatalf("json export missing ticket:\n%s", js)
	}

	outFile := filepath.Join(dir, "export.md")
	if _, err := run(t, dir, "export", "--out", outFile); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "exported bug") {
		t.Fatalf("export --out file missing content:\n%s", data)
	}
}

func TestContextAndPromptTextOutput(t *testing.T) {
	dir := initRepo(t)
	run(t, dir, "create", "--type", "bug", "--title", "context bug", "-p", "critical")

	ctx, err := run(t, dir, "context")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ctx, "context bug") {
		t.Fatalf("context missing ticket:\n%s", ctx)
	}

	outFile := filepath.Join(dir, "ctx.md")
	if _, err := run(t, dir, "context", "--out", outFile); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(outFile); err != nil || !strings.Contains(string(data), "context bug") {
		t.Fatalf("context --out missing content: err=%v data=%s", err, data)
	}

	prompt, err := run(t, dir, "prompt", "BUG-001")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "BUG-001") {
		t.Fatalf("prompt missing ticket id:\n%s", prompt)
	}
}

// --- init: pineIgnored warning ---

func TestInitWarnsWhenPineGitignored(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules\n.pine\n"), 0o644)
	out, err := run(t, dir, "init", "--skip-agents")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "gitignored") {
		t.Fatalf("expected gitignored warning:\n%s", out)
	}
}

func TestInitNoWarningWithoutGitignoreRule(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules\n"), 0o644)
	out, err := run(t, dir, "init", "--skip-agents")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "gitignored") {
		t.Fatalf("unexpected gitignored warning:\n%s", out)
	}
}

// --- setup command variants ---

func TestSetupBareErrorsWithHint(t *testing.T) {
	dir := initRepo(t)
	_, err := run(t, dir, "setup")
	if err == nil || !strings.Contains(err.Error(), "pine setup agent") {
		t.Fatalf("expected hint error, got %v", err)
	}
}

func TestSetupListPrintRemove(t *testing.T) {
	dir := initRepo(t)

	listOut, err := run(t, dir, "setup", "--list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listOut, "CLAUDE.md") {
		t.Fatalf("expected recipe list:\n%s", listOut)
	}

	printOut, err := run(t, dir, "setup", "claude", "--print")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(printOut, "pine:begin") {
		t.Fatalf("expected rendered template:\n%s", printOut)
	}

	checkOut, err := run(t, dir, "setup", "claude", "--check")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(checkOut, "missing") && !strings.Contains(checkOut, "not") {
		t.Fatalf("expected not-installed check status:\n%s", checkOut)
	}

	if _, err := run(t, dir, "setup", "agents"); err != nil {
		t.Fatal(err)
	}
	removeOut, err := run(t, dir, "setup", "--remove")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(removeOut, "AGENTS.md") {
		t.Fatalf("expected remove output:\n%s", removeOut)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if strings.Contains(string(data), "pine:begin") {
		t.Fatalf("pine section should be removed:\n%s", data)
	}
}

// --- learn search text output ---

func TestLearnSearchTextOutput(t *testing.T) {
	dir := initRepo(t)
	run(t, dir, "learn", "UNIQUE_TEXT_SEARCH_TERM about caching")

	noHits, err := run(t, dir, "learn", "search", "nothing_matches_this_zzz")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(noHits, "No learnings matched.") {
		t.Fatalf("expected no-hits message:\n%s", noHits)
	}

	hits, err := run(t, dir, "learn", "search", "UNIQUE_TEXT_SEARCH_TERM")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(hits, "ID") || !strings.Contains(hits, "SCORE") {
		t.Fatalf("expected search table:\n%s", hits)
	}
}

func TestLearnListAndSearchTicketFilter(t *testing.T) {
	dir := initRepo(t)
	run(t, dir, "create", "--type", "bug", "--title", "x") // BUG-001
	run(t, dir, "learn", "ticket-scoped filter test", "--scope", "ticket", "--ticket", "BUG-001")
	run(t, dir, "learn", "global one")

	listOut, err := run(t, dir, "learn", "list", "--ticket", "BUG-001")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listOut, "ticket-scoped filter test") || strings.Contains(listOut, "global one") {
		t.Fatalf("--ticket filter did not narrow the list:\n%s", listOut)
	}
}

// --- optimize: nothing to do ---

func TestOptimizeNothingToOptimize(t *testing.T) {
	dir := initRepo(t)
	out, err := run(t, dir, "optimize")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Nothing to optimize.") {
		t.Fatalf("expected nothing-to-optimize message:\n%s", out)
	}
	dryOut, err := run(t, dir, "optimize", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dryOut, "Nothing to optimize.") {
		t.Fatalf("expected nothing-to-optimize message on dry-run:\n%s", dryOut)
	}
}

// --- doctor: warning and error branches through the CLI ---

func TestDoctorCLIWarningAndError(t *testing.T) {
	dir := initRepo(t)
	os.WriteFile(filepath.Join(dir, ".pine", "tickets", "notes.txt"), []byte("scratch"), 0o644)
	warnOut, err := run(t, dir, "doctor")
	if err != nil {
		t.Fatalf("stray file alone should not error: %v\n%s", err, warnOut)
	}
	if !strings.Contains(warnOut, "!") {
		t.Fatalf("expected a warning symbol in doctor output:\n%s", warnOut)
	}

	os.WriteFile(filepath.Join(dir, ".pine", "tickets", "BUG-001.md"), []byte("no frontmatter\n"), 0o644)
	_, err = run(t, dir, "doctor")
	if err == nil || !strings.Contains(err.Error(), "problem") {
		t.Fatalf("expected doctor to report an error, got %v", err)
	}
}

package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/underworld14/pine/internal/config"
	"github.com/underworld14/pine/internal/ticket"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// scaffold creates a fresh .pine/ tree (sequential ids for deterministic
// BUG-001… assertions) and opens a store over it.
func scaffold(t *testing.T) *Store { return scaffoldStyle(t, "sequential") }

// scaffoldHash is scaffold with the default hash id style.
func scaffoldHash(t *testing.T) *Store { return scaffoldStyle(t, "hash") }

func scaffoldStyle(t *testing.T, style string) *Store {
	t.Helper()
	pine := filepath.Join(t.TempDir(), ".pine")
	must(t, os.MkdirAll(filepath.Join(pine, "tickets"), 0o755))
	cfg := config.Default("test")
	cfg.IDStyle = style
	cfgB, err := cfg.Bytes()
	must(t, err)
	must(t, os.WriteFile(filepath.Join(pine, "config.json"), cfgB, 0o644))
	bB, err := config.DefaultBoard().Bytes()
	must(t, err)
	must(t, os.WriteFile(filepath.Join(pine, "board.json"), bB, 0o644))
	s, err := Open(pine)
	must(t, err)
	return s
}

func TestCreateSeedsAcceptanceCriteria(t *testing.T) {
	s := scaffold(t)
	for _, typ := range []string{"BUG", "FEAT"} {
		tk, err := s.Create(CreateReq{Type: typ, Title: "x"})
		must(t, err)
		if !strings.Contains(tk.Body, "# Acceptance Criteria") {
			t.Errorf("%s body missing Acceptance Criteria:\n%s", typ, tk.Body)
		}
	}
}

func TestHashIDsUniqueAndValid(t *testing.T) {
	s := scaffoldHash(t)
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		tk, err := s.Create(CreateReq{Type: "bug", Title: "x"})
		must(t, err)
		if !ticket.ValidID(tk.ID) || ticket.PrefixOf(tk.ID) != "BUG" {
			t.Fatalf("bad hash id %q", tk.ID)
		}
		if seen[tk.ID] {
			t.Fatalf("duplicate hash id %q", tk.ID)
		}
		seen[tk.ID] = true
	}
}

func TestHashIDsAvoidCrossBranchCollision(t *testing.T) {
	// Two independent stores over separate .pine dirs stand in for two branches;
	// the same prefix on each must not yield the same filename.
	a := scaffoldHash(t)
	b := scaffoldHash(t)
	ta, err := a.Create(CreateReq{Type: "bug", Title: "on branch a"})
	must(t, err)
	tb, err := b.Create(CreateReq{Type: "bug", Title: "on branch b"})
	must(t, err)
	if ta.ID == tb.ID {
		t.Errorf("hash ids collided across branches: %q", ta.ID)
	}
}

func TestSetIDGenDeterministic(t *testing.T) {
	s := scaffoldHash(t)
	i := 0
	s.SetIDGen(func() string { i++; return fmt.Sprintf("aaaa%02d", i) })
	tk, err := s.Create(CreateReq{Type: "bug", Title: "x"})
	must(t, err)
	if tk.ID != "BUG-aaaa01" {
		t.Errorf("id = %q, want BUG-aaaa01", tk.ID)
	}
}

// steppingClock returns times that advance one second per call.
func steppingClock() func() time.Time {
	base := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
	i := 0
	return func() time.Time {
		i++
		return base.Add(time.Duration(i) * time.Second)
	}
}

func TestCreateSequentialIDs(t *testing.T) {
	s := scaffold(t)
	a, err := s.Create(CreateReq{Type: "bug", Title: "one"})
	must(t, err)
	b, err := s.Create(CreateReq{Type: "bug", Title: "two"})
	must(t, err)
	if a.ID != "BUG-001" || b.ID != "BUG-002" {
		t.Fatalf("ids = %s, %s", a.ID, b.ID)
	}
	if _, err := os.Stat(s.ticketPath("BUG-001")); err != nil {
		t.Errorf("file not written: %v", err)
	}
}

func TestCreateUsesTemplateAndTypeName(t *testing.T) {
	s := scaffold(t)
	f, err := s.Create(CreateReq{Type: "feature", Title: "x"})
	must(t, err)
	if f.ID != "FEAT-001" {
		t.Errorf("id = %s", f.ID)
	}
	if !strings.Contains(f.Body, "Acceptance Criteria") {
		t.Errorf("feature template not applied: %q", f.Body)
	}
	e, err := s.Create(CreateReq{Type: "epic", Title: "e"})
	must(t, err)
	if e.ID != "EPIC-001" || !strings.Contains(e.Body, "Goals") {
		t.Errorf("epic create = %s / %q", e.ID, e.Body)
	}
}

func TestCreateUnknownType(t *testing.T) {
	s := scaffold(t)
	if _, err := s.Create(CreateReq{Type: "zzz", Title: "x"}); err != ErrUnknownType {
		t.Errorf("err = %v", err)
	}
}

func TestUpdatePreservesBodyAndBumpsUpdated(t *testing.T) {
	s := scaffold(t)
	s.SetClock(steppingClock())
	tk, err := s.Create(CreateReq{Type: "bug", Title: "x", Body: "# Description\n\nhello\n"})
	must(t, err)
	origBody := tk.Body
	updated, err := s.Update(tk.ID, func(u *ticket.Ticket) error {
		u.Status = "done"
		return nil
	})
	must(t, err)
	if updated.Status != "done" {
		t.Errorf("status = %s", updated.Status)
	}
	if updated.Body != origBody {
		t.Errorf("body changed: %q vs %q", updated.Body, origBody)
	}
	if !updated.Updated.After(tk.Updated) {
		t.Errorf("updated not bumped: %v vs %v", updated.Updated, tk.Updated)
	}
}

func TestUpdateDegradedRejected(t *testing.T) {
	s := scaffold(t)
	// Write a malformed ticket file directly and reload it.
	must(t, os.WriteFile(s.ticketPath("BUG-001"), []byte("no frontmatter here\n"), 0o644))
	_, err := s.ReloadTicket(s.ticketPath("BUG-001"))
	must(t, err)
	if _, err := s.Update("BUG-001", func(u *ticket.Ticket) error { return nil }); err != ErrDegraded {
		t.Errorf("err = %v want ErrDegraded", err)
	}
}

func TestDeleteRemovesTicketAndAttachments(t *testing.T) {
	s := scaffold(t)
	tk, err := s.Create(CreateReq{Type: "bug", Title: "x"})
	must(t, err)
	if _, err := s.WriteAttachment(tk.ID, "a.png", []byte("img")); err != nil {
		t.Fatal(err)
	}
	if len(s.Attachments(tk.ID)) != 1 {
		t.Fatalf("expected 1 attachment")
	}
	must(t, s.Delete(tk.ID))
	if _, err := s.Get(tk.ID); err != ErrNotFound {
		t.Errorf("get err = %v", err)
	}
	if _, err := os.Stat(s.attachmentDir(tk.ID)); !os.IsNotExist(err) {
		t.Errorf("attachment dir not removed")
	}
}

func TestConcurrentCreateUniqueIDs(t *testing.T) {
	s := scaffold(t)
	const n = 20
	var wg sync.WaitGroup
	ids := make(chan string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if tk, err := s.Create(CreateReq{Type: "bug", Title: "x"}); err == nil {
				ids <- tk.ID
			}
		}()
	}
	wg.Wait()
	close(ids)
	seen := map[string]bool{}
	for id := range ids {
		if seen[id] {
			t.Errorf("duplicate id %s", id)
		}
		seen[id] = true
	}
	if len(seen) != n {
		t.Errorf("got %d unique ids, want %d", len(seen), n)
	}
}

func TestCreateSkipsExistingID(t *testing.T) {
	s := scaffold(t)
	// Simulate an externally-created ticket file occupying BUG-001.
	must(t, os.WriteFile(s.ticketPath("BUG-001"), []byte("x"), 0o644))
	tk, err := s.Create(CreateReq{Type: "bug", Title: "x"})
	must(t, err)
	if tk.ID == "BUG-001" {
		t.Errorf("should not reuse an existing id")
	}
}

func TestReloadTicketDedupe(t *testing.T) {
	s := scaffold(t)
	tk, err := s.Create(CreateReq{Type: "bug", Title: "x"})
	must(t, err)
	raw, err := os.ReadFile(s.ticketPath(tk.ID))
	must(t, err)
	edited := strings.Replace(string(raw), "status: todo", "status: done", 1)
	must(t, os.WriteFile(s.ticketPath(tk.ID), []byte(edited), 0o644))

	ch, err := s.ReloadTicket(s.ticketPath(tk.ID))
	must(t, err)
	if !ch.Changed {
		t.Errorf("expected change on external edit")
	}
	got, _ := s.Get(tk.ID)
	if got.Status != "done" {
		t.Errorf("status = %s", got.Status)
	}
	ch2, err := s.ReloadTicket(s.ticketPath(tk.ID))
	must(t, err)
	if ch2.Changed {
		t.Errorf("reload of unchanged file should dedupe")
	}
}

func TestHashChangesOnUpdate(t *testing.T) {
	s := scaffold(t)
	tk, err := s.Create(CreateReq{Type: "bug", Title: "x"})
	must(t, err)
	h1, _ := s.Hash(tk.ID)
	if _, err := s.Update(tk.ID, func(u *ticket.Ticket) error { u.Title = "y"; return nil }); err != nil {
		t.Fatal(err)
	}
	h2, _ := s.Hash(tk.ID)
	if h1 == h2 || h1 == "" || h2 == "" {
		t.Errorf("hash should change: %q -> %q", h1, h2)
	}
}

func TestUpdateIfMatchConflict(t *testing.T) {
	s := scaffold(t)
	tk, err := s.Create(CreateReq{Type: "bug", Title: "x"})
	must(t, err)
	h1, _ := s.Hash(tk.ID)

	// Correct hash succeeds and changes the hash.
	if _, err := s.UpdateIfMatch(tk.ID, h1, func(u *ticket.Ticket) error { u.Title = "a"; return nil }); err != nil {
		t.Fatal(err)
	}
	// The now-stale hash must be rejected as a conflict.
	if _, err := s.UpdateIfMatch(tk.ID, h1, func(u *ticket.Ticket) error { u.Title = "b"; return nil }); err != ErrConflict {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

func TestStatusNormalizedOnWrite(t *testing.T) {
	s := scaffold(t)
	tk, err := s.Create(CreateReq{Type: "bug", Title: "x", Status: "Doing"})
	must(t, err)
	if tk.Status != "doing" {
		t.Errorf("create status = %q, want lowercased", tk.Status)
	}
	up, err := s.Update(tk.ID, func(u *ticket.Ticket) error { u.Status = "DONE"; return nil })
	must(t, err)
	if up.Status != "done" {
		t.Errorf("update status = %q, want lowercased", up.Status)
	}
}

func TestListFilter(t *testing.T) {
	s := scaffold(t)
	if _, err := s.Create(CreateReq{Type: "bug", Title: "a", Labels: []string{"ui"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(CreateReq{Type: "feature", Title: "b"}); err != nil {
		t.Fatal(err)
	}
	if len(s.List(Filter{Type: "BUG"})) != 1 {
		t.Errorf("type filter")
	}
	if len(s.List(Filter{Label: "ui"})) != 1 {
		t.Errorf("label filter")
	}
	if len(s.All()) != 2 {
		t.Errorf("all")
	}
}

func TestConfigAndBoardGetters(t *testing.T) {
	s := scaffold(t)
	cfg := s.Config()
	if cfg == nil || cfg.Project.Name != "test" {
		t.Fatalf("Config() = %+v", cfg)
	}
	board := s.Board()
	if board == nil || len(board.Columns) == 0 {
		t.Fatalf("Board() = %+v", board)
	}
}

func TestGraphReflectsDepsAndParent(t *testing.T) {
	s := scaffold(t)
	epic, err := s.Create(CreateReq{Type: "epic", Title: "e"})
	must(t, err)
	blocker, err := s.Create(CreateReq{Type: "bug", Title: "blocker", Parent: epic.ID})
	must(t, err)
	blocked, err := s.Create(CreateReq{Type: "bug", Title: "blocked", Deps: []string{blocker.ID}})
	must(t, err)

	g := s.Graph()
	if !g.Blocked(blocked.ID) {
		t.Errorf("expected %s to be blocked by unmet dep %s", blocked.ID, blocker.ID)
	}
	kids := g.Children(epic.ID)
	if len(kids) != 1 || kids[0].ID != blocker.ID {
		t.Errorf("children = %#v", kids)
	}
	if len(g.Cycles()) != 0 {
		t.Errorf("expected no cycles, got %v", g.Cycles())
	}
}

func TestSortByPriorityThenUpdated(t *testing.T) {
	s := scaffold(t)
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	ts := []*ticket.Ticket{
		{ID: "BUG-001", Priority: "low", Updated: base},
		{ID: "BUG-002", Priority: "critical", Updated: base},
		{ID: "BUG-003", Priority: "critical", Updated: base.Add(time.Hour)},
		{ID: "BUG-004", Priority: "medium", Updated: base},
	}
	s.SortByPriorityThenUpdated(ts)
	want := []string{"BUG-003", "BUG-002", "BUG-004", "BUG-001"}
	var got []string
	for _, t := range ts {
		got = append(got, t.ID)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestAttachmentDirs(t *testing.T) {
	s := scaffold(t)
	if dirs := s.AttachmentDirs(); dirs != nil {
		t.Errorf("expected nil for missing attachments root, got %v", dirs)
	}
	tk, err := s.Create(CreateReq{Type: "bug", Title: "x"})
	must(t, err)
	if _, err := s.WriteAttachment(tk.ID, "a.png", []byte("img")); err != nil {
		t.Fatal(err)
	}
	tk2, err := s.Create(CreateReq{Type: "bug", Title: "y"})
	must(t, err)
	if _, err := s.WriteAttachment(tk2.ID, "b.png", []byte("img")); err != nil {
		t.Fatal(err)
	}
	// A dot-prefixed entry (e.g. .DS_Store) must be excluded.
	must(t, os.MkdirAll(filepath.Join(s.attachmentsRoot(), ".hidden"), 0o755))

	dirs := s.AttachmentDirs()
	if len(dirs) != 2 || dirs[0] != tk.ID || dirs[1] != tk2.ID {
		t.Errorf("AttachmentDirs() = %v, want [%s %s]", dirs, tk.ID, tk2.ID)
	}
}

func TestReloadConfigNoopAndChange(t *testing.T) {
	s := scaffold(t)
	cfgPath := filepath.Join(s.Root(), "config.json")

	changed, err := s.ReloadConfig()
	must(t, err)
	if changed {
		t.Errorf("echo of identical config should not report change")
	}

	raw, err := os.ReadFile(cfgPath)
	must(t, err)
	edited := strings.Replace(string(raw), `"test"`, `"renamed"`, 1)
	must(t, os.WriteFile(cfgPath, []byte(edited), 0o644))

	changed, err = s.ReloadConfig()
	must(t, err)
	if !changed {
		t.Fatal("expected change after editing config.json")
	}
	if s.Config().Project.Name != "renamed" {
		t.Errorf("project name = %q", s.Config().Project.Name)
	}
}

func TestReloadConfigMalformed(t *testing.T) {
	s := scaffold(t)
	cfgPath := filepath.Join(s.Root(), "config.json")
	must(t, os.WriteFile(cfgPath, []byte("not json"), 0o644))
	if _, err := s.ReloadConfig(); err == nil {
		t.Fatal("expected error reloading malformed config")
	}
}

func TestReloadBoardNoopAndChange(t *testing.T) {
	s := scaffold(t)
	boardPath := filepath.Join(s.Root(), "board.json")

	changed, err := s.ReloadBoard()
	must(t, err)
	if changed {
		t.Errorf("echo of identical board should not report change")
	}

	raw, err := os.ReadFile(boardPath)
	must(t, err)
	edited := strings.Replace(string(raw), "]", `,{"id":"extra","name":"Extra"}]`, 1)
	must(t, os.WriteFile(boardPath, []byte(edited), 0o644))

	changed, err = s.ReloadBoard()
	must(t, err)
	if !changed {
		t.Fatal("expected change after editing board.json")
	}
}

func TestReloadBoardMalformed(t *testing.T) {
	s := scaffold(t)
	boardPath := filepath.Join(s.Root(), "board.json")
	must(t, os.WriteFile(boardPath, []byte("not json"), 0o644))
	if _, err := s.ReloadBoard(); err == nil {
		t.Fatal("expected error reloading malformed board")
	}
}

func TestSaveConfigWritesToDisk(t *testing.T) {
	s := scaffold(t)
	cfg := cloneConfig(t, s)
	cfg.Project.Name = "updated-name"
	must(t, s.SaveConfig(cfg))

	if s.Config().Project.Name != "updated-name" {
		t.Errorf("in-memory config not updated: %q", s.Config().Project.Name)
	}
	raw, err := os.ReadFile(filepath.Join(s.Root(), "config.json"))
	must(t, err)
	if !strings.Contains(string(raw), "updated-name") {
		t.Errorf("config.json not written: %s", raw)
	}
}

func TestSaveConfigInvalidRejected(t *testing.T) {
	s := scaffold(t)
	cfg := cloneConfig(t, s)
	cfg.Version = 0
	if err := s.SaveConfig(cfg); err == nil {
		t.Fatal("expected validation error for version 0")
	}
	if s.Config().Version == 0 {
		t.Errorf("invalid config must not be applied in-memory")
	}
}

// cloneConfig returns an independent, mutable copy of the store's current
// config via a JSON round-trip (Config has no exported clone method).
func cloneConfig(t *testing.T, s *Store) *config.Config {
	t.Helper()
	b, err := s.Config().Bytes()
	must(t, err)
	cfg, err := config.Parse(b)
	must(t, err)
	return cfg
}

func TestLoadTicketFileOnReopenAndScanSkips(t *testing.T) {
	s := scaffold(t)
	tk, err := s.Create(CreateReq{Type: "bug", Title: "x"})
	must(t, err)
	// A subdirectory and a non-matching filename in tickets/ must be skipped by scan.
	must(t, os.MkdirAll(filepath.Join(s.ticketsDir(), "subdir"), 0o755))
	must(t, os.WriteFile(filepath.Join(s.ticketsDir(), "README.txt"), []byte("hi"), 0o644))

	reopened, err := Open(s.Root())
	must(t, err)
	got, err := reopened.Get(tk.ID)
	must(t, err)
	if got.Title != "x" {
		t.Errorf("reloaded ticket title = %q", got.Title)
	}
	if len(reopened.All()) != 1 {
		t.Errorf("expected exactly 1 ticket after scan, got %d: %#v", len(reopened.All()), reopened.All())
	}
}

func TestScanTicketsMissingDirIsNotError(t *testing.T) {
	pine := filepath.Join(t.TempDir(), ".pine")
	must(t, os.MkdirAll(pine, 0o755))
	cfg := config.Default("test")
	cfgB, err := cfg.Bytes()
	must(t, err)
	must(t, os.WriteFile(filepath.Join(pine, "config.json"), cfgB, 0o644))
	bB, err := config.DefaultBoard().Bytes()
	must(t, err)
	must(t, os.WriteFile(filepath.Join(pine, "board.json"), bB, 0o644))
	// Note: no tickets/ subdirectory is created.
	s, err := Open(pine)
	must(t, err)
	if len(s.All()) != 0 {
		t.Errorf("expected no tickets, got %d", len(s.All()))
	}
}

func TestDeleteAttachmentBranches(t *testing.T) {
	s := scaffold(t)
	tk, err := s.Create(CreateReq{Type: "bug", Title: "x"})
	must(t, err)
	if _, err := s.WriteAttachment(tk.ID, "a.png", []byte("img")); err != nil {
		t.Fatal(err)
	}
	// Success path.
	must(t, s.DeleteAttachment(tk.ID, "a.png"))
	if len(s.Attachments(tk.ID)) != 0 {
		t.Errorf("attachment not deleted")
	}
	// Deleting a file that no longer exists is a no-op, not an error.
	if err := s.DeleteAttachment(tk.ID, "a.png"); err != nil {
		t.Errorf("delete of missing file should be nil, got %v", err)
	}
	// Invalid ticket id.
	if err := s.DeleteAttachment("not-an-id", "a.png"); err == nil {
		t.Errorf("expected error for invalid ticket id")
	}
	// Invalid filename (dotfiles are rejected outright, not just skipped).
	if err := s.DeleteAttachment(tk.ID, ".hidden"); err == nil {
		t.Errorf("expected error for invalid filename")
	}
}

func TestMimeAndKindAllExtensions(t *testing.T) {
	cases := map[string][2]string{
		"a.png":     {"image/png", "image"},
		"a.jpg":     {"image/jpeg", "image"},
		"a.jpeg":    {"image/jpeg", "image"},
		"a.gif":     {"image/gif", "image"},
		"a.webp":    {"image/webp", "image"},
		"a.mp4":     {"video/mp4", "video"},
		"a.mov":     {"video/quicktime", "video"},
		"a.PNG":     {"image/png", "image"},
		"a.unknown": {"application/octet-stream", "other"},
		"noext":     {"application/octet-stream", "other"},
	}
	for name, want := range cases {
		mime, kind := MimeAndKind(name)
		if mime != want[0] || kind != want[1] {
			t.Errorf("MimeAndKind(%q) = %q, %q; want %q, %q", name, mime, kind, want[0], want[1])
		}
	}
}

func TestSanitizeNameRejectsBadNames(t *testing.T) {
	// These fail outright: the base name itself (after filepath.Base) is
	// empty/"."/".." , contains a literal backslash, or starts with a dot.
	bad := []string{"", ".", "..", `a\b`, ".hidden", "a..b"}
	for _, name := range bad {
		if _, err := sanitizeName(name); err == nil {
			t.Errorf("sanitizeName(%q) should have been rejected", name)
		}
	}
	// A path with directory components is reduced to its safe basename rather
	// than rejected — filepath.Base already strips the ".." segments.
	got, err := sanitizeName("../escape/ok.png")
	must(t, err)
	if got != "ok.png" {
		t.Errorf("sanitizeName(%q) = %q, want reduced basename", "../escape/ok.png", got)
	}
	got, err = sanitizeName("  ok.png  ")
	must(t, err)
	if got != "ok.png" {
		t.Errorf("sanitizeName trimmed = %q", got)
	}
}

func TestWriteAttachmentInvalidID(t *testing.T) {
	s := scaffold(t)
	if _, err := s.WriteAttachment("not-an-id", "a.png", []byte("x")); err == nil {
		t.Fatal("expected error for invalid ticket id")
	}
}

func TestWriteAttachmentInvalidName(t *testing.T) {
	s := scaffold(t)
	tk, err := s.Create(CreateReq{Type: "bug", Title: "x"})
	must(t, err)
	if _, err := s.WriteAttachment(tk.ID, ".hidden", []byte("x")); err == nil {
		t.Fatal("expected error for invalid attachment filename")
	}
}

func TestAttachmentsOnMissingDirReturnsNil(t *testing.T) {
	s := scaffold(t)
	tk, err := s.Create(CreateReq{Type: "bug", Title: "x"})
	must(t, err)
	if got := s.Attachments(tk.ID); got != nil {
		t.Errorf("Attachments() for a ticket with no attachments dir = %v, want nil", got)
	}
}

func TestAttachmentsSkipsDirsAndDotfiles(t *testing.T) {
	s := scaffold(t)
	tk, err := s.Create(CreateReq{Type: "bug", Title: "x"})
	must(t, err)
	if _, err := s.WriteAttachment(tk.ID, "keep.png", []byte("img")); err != nil {
		t.Fatal(err)
	}
	must(t, os.MkdirAll(filepath.Join(s.attachmentDir(tk.ID), "subdir"), 0o755))
	must(t, os.WriteFile(filepath.Join(s.attachmentDir(tk.ID), ".DS_Store"), []byte("x"), 0o644))

	got := s.Attachments(tk.ID)
	if len(got) != 1 || got[0].Name != "keep.png" {
		t.Errorf("Attachments() = %#v, want only keep.png", got)
	}
}

func TestAttachmentFilePathInvalidName(t *testing.T) {
	s := scaffold(t)
	if _, err := s.AttachmentFilePath("BUG-001", ".hidden"); err == nil {
		t.Fatal("expected error for invalid attachment filename")
	}
}

func TestReloadTicketNonMatchingFilename(t *testing.T) {
	s := scaffold(t)
	ch, err := s.ReloadTicket(filepath.Join(s.ticketsDir(), "README.txt"))
	must(t, err)
	if ch != (Change{}) {
		t.Errorf("expected zero-value Change for non-matching filename, got %+v", ch)
	}
}

func TestReloadTicketRemoval(t *testing.T) {
	s := scaffold(t)
	tk, err := s.Create(CreateReq{Type: "bug", Title: "x"})
	must(t, err)
	path := s.ticketPath(tk.ID)
	must(t, os.Remove(path))

	ch, err := s.ReloadTicket(path)
	must(t, err)
	if !ch.Removed || !ch.Changed {
		t.Fatalf("expected removed+changed, got %+v", ch)
	}
	if _, err := s.Get(tk.ID); err != ErrNotFound {
		t.Errorf("get after removal err = %v", err)
	}
	// Reloading an already-absent file again should report no existing entry.
	ch2, err := s.ReloadTicket(path)
	must(t, err)
	if ch2.Removed || ch2.Changed {
		t.Errorf("second reload of already-removed file: %+v", ch2)
	}
}

func TestApplyLearningMtimeFallbackFillsFromDisk(t *testing.T) {
	s := scaffold(t)
	path := filepath.Join(s.learningsDir(), "LRN-001.md")
	must(t, os.MkdirAll(filepath.Dir(path), 0o755))
	must(t, os.WriteFile(path, []byte(`---
id: LRN-001
scope: global
source_agent: manual
---
no created field
`), 0o644))
	reopened, err := Open(s.Root())
	must(t, err)
	l, err := reopened.GetLearning("LRN-001")
	must(t, err)
	if l.Created.IsZero() {
		t.Errorf("expected Created filled from mtime, got zero")
	}
}

func TestOpenRejectsMalformedConfig(t *testing.T) {
	pine := filepath.Join(t.TempDir(), ".pine")
	must(t, os.MkdirAll(pine, 0o755))
	must(t, os.WriteFile(filepath.Join(pine, "config.json"), []byte("not json"), 0o644))
	bB, err := config.DefaultBoard().Bytes()
	must(t, err)
	must(t, os.WriteFile(filepath.Join(pine, "board.json"), bB, 0o644))
	if _, err := Open(pine); err == nil {
		t.Fatal("expected error opening store with malformed config.json")
	}
}

func TestOpenRejectsMalformedBoard(t *testing.T) {
	pine := filepath.Join(t.TempDir(), ".pine")
	must(t, os.MkdirAll(pine, 0o755))
	cfg := config.Default("test")
	cfgB, err := cfg.Bytes()
	must(t, err)
	must(t, os.WriteFile(filepath.Join(pine, "config.json"), cfgB, 0o644))
	must(t, os.WriteFile(filepath.Join(pine, "board.json"), []byte("not json"), 0o644))
	if _, err := Open(pine); err == nil {
		t.Fatal("expected error opening store with malformed board.json")
	}
}

func TestTemplateUsesCustomFileWhenPresent(t *testing.T) {
	s := scaffold(t)
	must(t, os.MkdirAll(filepath.Join(s.Root(), "templates"), 0o755))
	must(t, os.WriteFile(filepath.Join(s.Root(), "templates", "bug.md"), []byte("Custom bug template\n"), 0o644))
	tk, err := s.Create(CreateReq{Type: "bug", Title: "x"})
	must(t, err)
	if !strings.Contains(tk.Body, "Custom bug template") {
		t.Errorf("body = %q, want custom template applied", tk.Body)
	}
}

func TestAllocHashInExhaustsAttempts(t *testing.T) {
	s := scaffoldHash(t)
	// A constant id generator forces every attempt after the first to collide.
	s.SetIDGen(func() string { return "aaaaaa" })
	if _, err := s.Create(CreateReq{Type: "bug", Title: "first"}); err != nil {
		t.Fatal(err)
	}
	_, err := s.Create(CreateReq{Type: "bug", Title: "second"})
	if err == nil || !strings.Contains(err.Error(), "could not allocate a unique") {
		t.Fatalf("expected exhaustion error, got %v", err)
	}
}

func TestAttachmentPathTraversalNeutralized(t *testing.T) {
	s := scaffold(t)
	// A traversal attempt must never resolve outside the ticket's attachment dir.
	// It may be rejected outright or collapsed to a safe basename; either is fine.
	if p, err := s.AttachmentFilePath("BUG-001", "../../etc/passwd"); err == nil {
		if !strings.HasPrefix(p, s.attachmentDir("BUG-001")+string(filepath.Separator)) {
			t.Errorf("path escaped attachments dir: %s", p)
		}
	}
	if _, err := s.AttachmentFilePath("BUG-001", "ok.png"); err != nil {
		t.Errorf("valid name rejected: %v", err)
	}
	if _, err := s.AttachmentFilePath("bad-id", "ok.png"); err == nil {
		t.Errorf("invalid ticket id should be rejected")
	}
	if err := s.DeleteAttachment("..", "config.yaml"); err == nil {
		t.Errorf("DeleteAttachment with invalid id should be rejected")
	}
}

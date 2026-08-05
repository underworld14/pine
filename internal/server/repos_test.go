package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/underworld14/pine/internal/config"
	"github.com/underworld14/pine/internal/store"
)

func openTestStore(t *testing.T, name string) *store.Store {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	pine := filepath.Join(root, ".pine")
	if err := os.MkdirAll(filepath.Join(pine, "tickets"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default(name)
	cfg.IDStyle = "sequential"
	cfgB, _ := cfg.Bytes()
	os.WriteFile(filepath.Join(pine, "config.json"), cfgB, 0o644)
	bB, _ := config.DefaultBoard().Bytes()
	os.WriteFile(filepath.Join(pine, "board.json"), bB, 0o644)
	st, err := store.Open(pine)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func newMultiRepoServer(t *testing.T) *httptest.Server {
	t.Helper()
	api := openTestStore(t, "api")
	web := openTestStore(t, "web")
	reg, err := NewRegistry([]string{"api", "web"}, map[string]*store.Store{
		"api": api,
		"web": web,
	}, map[string]string{
		"api": filepath.Dir(api.Root()),
		"web": filepath.Dir(web.Root()),
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(NewWithRegistry(reg, "test").Handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestMultiRepoHealthAndList(t *testing.T) {
	ts := newMultiRepoServer(t)
	resp, body := do(t, "GET", ts.URL+"/api/health", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"active":"api"`) {
		t.Fatalf("health = %s", body)
	}
	resp, body = do(t, "GET", ts.URL+"/api/repos", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	var payload struct {
		Active string              `json:"active"`
		Repos  []map[string]string `json:"repos"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Active != "api" || len(payload.Repos) != 2 {
		t.Fatalf("repos payload = %+v", payload)
	}
}

// TestNewRegistryErrors covers the NewRegistry validation paths: empty order
// and a missing store for a declared alias.
func TestNewRegistryErrors(t *testing.T) {
	if _, err := NewRegistry(nil, nil, nil); err == nil {
		t.Fatal("NewRegistry(empty) should error")
	}
	api := openTestStore(t, "api")
	if _, err := NewRegistry([]string{"api", "web"}, map[string]*store.Store{"api": api}, nil); err == nil {
		t.Fatal("NewRegistry with missing web store should error")
	}
}

// TestRegistryAccessor covers the Registry() accessor and confirms it exposes
// the registered aliases and the active one.
func TestRegistryAccessor(t *testing.T) {
	api := openTestStore(t, "api")
	web := openTestStore(t, "web")
	reg, err := NewRegistry([]string{"api", "web"}, map[string]*store.Store{"api": api, "web": web}, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := NewWithRegistry(reg, "test")
	if s.Registry() == nil {
		t.Fatal("Registry() nil")
	}
	if got := s.Registry().All(); len(got) != 2 || got[0] != "api" || got[1] != "web" {
		t.Fatalf("Registry().All() = %v", got)
	}
	if s.Registry().ActiveAlias() != "api" {
		t.Fatalf("active = %q want api", s.Registry().ActiveAlias())
	}
}

// TestGitForAliasRoute covers gitFor's non-active branch: a GET to an
// alias-routed git endpoint for a non-active repo takes a fresh snapshot
// (instead of the active-store cache).
func TestGitForAliasRoute(t *testing.T) {
	ts := newMultiRepoServer(t)
	// web is not the active repo (api is), so /api/r/web/git exercises the
	// non-active gitFor branch.
	resp, body := do(t, "GET", ts.URL+"/api/r/web/git", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("alias git status %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"isRepo"`) {
		t.Fatalf("alias git body missing status fields: %s", body)
	}
}

// TestActivateRepo covers handleActivateRepo's success and error branches:
// activating a registered alias swaps the active store; an unknown alias 404s.
func TestActivateRepo(t *testing.T) {
	ts := newMultiRepoServer(t)
	resp, body := do(t, "POST", ts.URL+"/api/repos/web/activate", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("activate web status %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"active":"web"`) {
		t.Fatalf("activate response = %s", body)
	}
	// The legacy /api/health now reports web as active.
	resp, body = do(t, "GET", ts.URL+"/api/health", "", nil)
	if !strings.Contains(body, `"active":"web"`) {
		t.Fatalf("health after activate = %s", body)
	}
	// Activating an unknown alias 404s.
	resp, _ = do(t, "POST", ts.URL+"/api/repos/nope/activate", "", nil)
	if resp.StatusCode != 404 {
		t.Fatalf("activate unknown should 404, got %d", resp.StatusCode)
	}
}

func TestMultiRepoActivateAndAliasRouting(t *testing.T) {
	ts := newMultiRepoServer(t)

	// Create a ticket in the active (api) store via legacy route.
	do(t, "POST", ts.URL+"/api/tickets", `{"type":"bug","title":"api bug"}`, nil)

	// Create a ticket in web via alias route.
	resp, body := do(t, "POST", ts.URL+"/api/r/web/tickets", `{"type":"bug","title":"web bug"}`, nil)
	if resp.StatusCode != 201 {
		t.Fatalf("create via alias status %d: %s", resp.StatusCode, body)
	}

	// Activate web.
	resp, body = do(t, "POST", ts.URL+"/api/repos/web/activate", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("activate status %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"active":"web"`) {
		t.Fatalf("activate body = %s", body)
	}

	// Alias snapshot for api should still have the first bug only.
	_, apiSnap := do(t, "GET", ts.URL+"/api/r/api/snapshot", "", nil)
	if !strings.Contains(apiSnap, "api bug") {
		t.Fatalf("api snapshot missing api bug: %s", apiSnap)
	}
	if strings.Contains(apiSnap, "web bug") {
		t.Fatalf("api snapshot should not include web ticket: %s", apiSnap)
	}

	_, webSnap := do(t, "GET", ts.URL+"/api/r/web/snapshot", "", nil)
	if !strings.Contains(webSnap, "web bug") {
		t.Fatalf("web snapshot missing web bug: %s", webSnap)
	}
	if strings.Contains(webSnap, "api bug") {
		t.Fatalf("web snapshot should not include api ticket: %s", webSnap)
	}

	// Unknown alias → 404.
	resp, _ = do(t, "GET", ts.URL+"/api/r/nope/snapshot", "", nil)
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404 for unknown alias, got %d", resp.StatusCode)
	}
}

func TestMultiRepoGraphAlias(t *testing.T) {
	ts := newMultiRepoServer(t)
	do(t, "POST", ts.URL+"/api/tickets", `{"type":"bug","title":"linked","links":["MEMORY"]}`, nil)
	resp, body := do(t, "GET", ts.URL+"/api/r/api/graph", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"nodes"`) || !strings.Contains(body, `"edges"`) {
		t.Fatalf("graph body = %s", body)
	}
}

// TestMultiRepoMutationRouting verifies the C1 fix: PUT/PATCH/DELETE via
// /api/r/{alias}/... must mutate the ALIAS store, not the active store, and SSE
// events must be tagged with the alias repo.
func TestMultiRepoMutationRouting(t *testing.T) {
	ts := newMultiRepoServer(t)

	// Seed a ticket in each repo via alias routes.
	_, apiBody := do(t, "POST", ts.URL+"/api/r/api/tickets", `{"type":"bug","title":"api bug"}`, nil)
	_, webBody := do(t, "POST", ts.URL+"/api/r/web/tickets", `{"type":"bug","title":"web bug"}`, nil)
	apiID := ticketIDFromJSON(t, apiBody)
	webID := ticketIDFromJSON(t, webBody)

	// PATCH web's ticket via alias route; must NOT touch api's ticket.
	resp, body := do(t, "PATCH", ts.URL+"/api/r/web/tickets/"+webID, `{"status":"done"}`, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("patch web status %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"status":"done"`) {
		t.Fatalf("patch response should show done: %s", body)
	}

	// api's ticket must be unchanged (still open/todo), web's must be done.
	_, apiGet := do(t, "GET", ts.URL+"/api/r/api/tickets/"+apiID, "", nil)
	if strings.Contains(apiGet, `"status":"done"`) {
		t.Fatalf("api ticket was mutated by a web-routed patch (C1 regression): %s", apiGet)
	}
	_, webGet := do(t, "GET", ts.URL+"/api/r/web/tickets/"+webID, "", nil)
	if !strings.Contains(webGet, `"status":"done"`) {
		t.Fatalf("web ticket not updated: %s", webGet)
	}

	// PUT web's config via alias route; must NOT change api's config.
	resp, body = do(t, "PUT", ts.URL+"/api/r/web/config", `{"project":{"name":"web-renamed"}}`, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("put web config status %d: %s", resp.StatusCode, body)
	}
	_, apiCfg := do(t, "GET", ts.URL+"/api/r/api/config", "", nil)
	if strings.Contains(apiCfg, "web-renamed") {
		t.Fatalf("api config was mutated by a web-routed put (C1 regression): %s", apiCfg)
	}
	_, webCfg := do(t, "GET", ts.URL+"/api/r/web/config", "", nil)
	if !strings.Contains(webCfg, "web-renamed") {
		t.Fatalf("web config not updated: %s", webCfg)
	}

	// DELETE web's ticket via alias route; api's ticket must survive.
	resp, _ = do(t, "DELETE", ts.URL+"/api/r/web/tickets/"+webID+"?opId=x", "", nil)
	if resp.StatusCode != 204 {
		t.Fatalf("delete web status %d", resp.StatusCode)
	}
	resp, _ = do(t, "GET", ts.URL+"/api/r/web/tickets/"+webID, "", nil)
	if resp.StatusCode != 404 {
		t.Fatalf("web ticket should be gone, got %d", resp.StatusCode)
	}
	resp, _ = do(t, "GET", ts.URL+"/api/r/api/tickets/"+apiID, "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("api ticket should survive a web-routed delete (C1 regression), got %d", resp.StatusCode)
	}
}

// TestMultiRepoSSETagging verifies the C2 fix: a mutation via /api/r/{alias}/...
// emits an SSE event tagged with that alias, not the active repo.
func TestMultiRepoSSETagging(t *testing.T) {
	ts := newMultiRepoServer(t)

	// Subscribe to the event stream.
	events := make(chan string, 16)
	req, _ := http.NewRequest("GET", ts.URL+"/api/events", nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req = req.WithContext(ctx)
	go func() {
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		buf := make([]byte, 0, 4096)
		tmp := make([]byte, 1024)
		for {
			n, err := resp.Body.Read(tmp)
			if n > 0 {
				buf = append(buf, tmp[:n]...)
				for {
					idx := bytes.IndexByte(buf, '\n')
					if idx < 0 {
						break
					}
					line := string(buf[:idx])
					buf = buf[idx+1:]
					if strings.HasPrefix(line, "data:") {
						events <- strings.TrimSpace(strings.TrimPrefix(line, "data:"))
					}
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// Give the SSE reader a moment to connect, then create a ticket in web.
	time.Sleep(150 * time.Millisecond)
	do(t, "POST", ts.URL+"/api/r/web/tickets", `{"type":"bug","title":"web sse bug"}`, nil)

	// Collect events for up to 2s looking for a ticket.created tagged repo=web.
	deadline := time.After(2 * time.Second)
	var sawWebTag bool
	for !sawWebTag {
		select {
		case data := <-events:
			if strings.Contains(data, `"repo":"web"`) && strings.Contains(data, `"ticket.created"`) || (strings.Contains(data, `"repo":"web"`) && strings.Contains(data, "web sse bug")) {
				sawWebTag = true
			}
		case <-deadline:
			t.Fatalf("did not see a repo=web ticket.created SSE event; last events drained")
		}
	}
	if !sawWebTag {
		t.Fatalf("expected an SSE event tagged repo=web for the alias-routed create")
	}
}

// TestMultiRepoAttachmentAlias verifies the I4 fix: attachments are served
// from the request's store, so /api/r/{alias}/attachments/{id}/{name} resolves
// the alias repo's file and the legacy /attachments/... serves the active store.
func TestMultiRepoAttachmentAlias(t *testing.T) {
	api := openTestStore(t, "api")
	web := openTestStore(t, "web")
	reg, err := NewRegistry([]string{"api", "web"}, map[string]*store.Store{
		"api": api, "web": web,
	}, map[string]string{
		"api": filepath.Dir(api.Root()),
		"web": filepath.Dir(web.Root()),
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(NewWithRegistry(reg, "test").Handler())
	defer ts.Close()

	// Seed a ticket in web, then drop an attachment file on disk directly.
	_, body := do(t, "POST", ts.URL+"/api/r/web/tickets", `{"type":"bug","title":"web attach"}`, nil)
	webID := ticketIDFromJSON(t, body)
	attDir := filepath.Join(web.Root(), "attachments", webID)
	if err := os.MkdirAll(attDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(attDir, "note.txt"), []byte("web-only attachment"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Fetch via web alias route → 200 + web's content.
	resp, body := do(t, "GET", ts.URL+"/api/r/web/attachments/"+webID+"/note.txt", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("alias attachment status %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "web-only attachment") {
		t.Fatalf("alias attachment body = %s", body)
	}

	// Fetch the same id via api alias route → 404 (api has no such attachment).
	resp, _ = do(t, "GET", ts.URL+"/api/r/api/attachments/"+webID+"/note.txt", "", nil)
	if resp.StatusCode != 404 {
		t.Fatalf("api alias should not serve web's attachment, got %d", resp.StatusCode)
	}

	// Legacy /attachments/{id}/{name} serves the active store (api) → 404 for web's id.
	resp, _ = do(t, "GET", ts.URL+"/attachments/"+webID+"/note.txt", "", nil)
	if resp.StatusCode != 404 {
		t.Fatalf("legacy attachment route should serve active store (api), got %d", resp.StatusCode)
	}

	// Upload via the web alias route → must land in web's store, NOT api's.
	ct, upBody := multipartImage(t, "files", "shot.png", pngBytes(8, 8))
	upReq, _ := http.NewRequest("POST", ts.URL+"/api/r/web/tickets/"+webID+"/attachments", bytes.NewReader(upBody))
	upReq.Header.Set("Content-Type", ct)
	upResp, err := http.DefaultClient.Do(upReq)
	if err != nil {
		t.Fatal(err)
	}
	upRespBody := readAll(t, upResp)
	if upResp.StatusCode != 201 {
		t.Fatalf("alias upload status %d: %s", upResp.StatusCode, upRespBody)
	}
	var up struct {
		Attachments []attachResult `json:"attachments"`
	}
	if err := json.Unmarshal([]byte(upRespBody), &up); err != nil || len(up.Attachments) != 1 {
		t.Fatalf("alias upload response: %s (%v)", upRespBody, err)
	}
	uploaded := up.Attachments[0].Name

	// The uploaded file is visible via web's alias route…
	resp, _ = do(t, "GET", ts.URL+"/api/r/web/attachments/"+webID+"/"+uploaded, "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("web alias should serve the uploaded file, got %d", resp.StatusCode)
	}
	// …and NOT via api's alias route (proves the upload landed in web, not the active api store).
	resp, _ = do(t, "GET", ts.URL+"/api/r/api/attachments/"+webID+"/"+uploaded, "", nil)
	if resp.StatusCode != 404 {
		t.Fatalf("api alias should not serve web's uploaded file, got %d (upload routed to wrong store)", resp.StatusCode)
	}

	// Delete via the web alias route → 204, then web can no longer serve it.
	delReq, _ := http.NewRequest("DELETE", ts.URL+"/api/r/web/tickets/"+webID+"/attachments/"+uploaded, nil)
	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatal(err)
	}
	if delResp.StatusCode != 204 {
		t.Fatalf("alias delete status %d", delResp.StatusCode)
	}
	resp, _ = do(t, "GET", ts.URL+"/api/r/web/attachments/"+webID+"/"+uploaded, "", nil)
	if resp.StatusCode != 404 {
		t.Fatalf("web alias should 404 after delete, got %d", resp.StatusCode)
	}
}

// ticketIDFromJSON extracts the "id" field from a JSON ticket response.
func ticketIDFromJSON(t *testing.T, body string) string {
	t.Helper()
	var v struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("ticket json: %v (%s)", err, body)
	}
	if v.ID == "" {
		t.Fatalf("no id in %s", body)
	}
	return v.ID
}

// TestSetActiveRepoConcurrentWithPollers exercises the I3 race: SetActiveRepo
// swaps srv.store / srv.git / srv.search / cross-branch fields while the git
// and cross-branch pollers read them. Run with -race to confirm the guards hold.
func TestSetActiveRepoConcurrentWithPollers(t *testing.T) {
	api := openTestStore(t, "api")
	web := openTestStore(t, "web")
	reg, err := NewRegistry([]string{"api", "web"}, map[string]*store.Store{
		"api": api, "web": web,
	}, map[string]string{
		"api": filepath.Dir(api.Root()),
		"web": filepath.Dir(web.Root()),
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := NewWithRegistry(reg, "test")
	stop := srv.StartLiveSync()
	defer stop()

	// Hammer SetActiveRepo from a goroutine while pollers tick and a reader
	// queries /api/git and /api/search on the active store.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			alias := "api"
			if i%2 == 1 {
				alias = "web"
			}
			_ = srv.SetActiveRepo(alias)
		}
	}()

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	for i := 0; i < 200; i++ {
		do(t, "GET", ts.URL+"/api/git", "", nil)
		do(t, "GET", ts.URL+"/api/search?q=x", "", nil)
	}
	<-done
}

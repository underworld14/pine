package server

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/underworld14/pine/internal/config"
	"github.com/underworld14/pine/internal/store"
)

// gitInitRepo creates a real git repo (with an initial commit so HEAD exists)
// plus a .pine tree, and opens a store over it. Used by the per-repo git poller
// test so gitx.Snapshot returns meaningful state that changes on a new commit.
func gitInitRepo(t *testing.T, name string) *store.Store {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v in %s: %v\n%s", args, root, err, out)
		}
	}
	run("init")
	run("config", "user.name", "test")
	run("config", "user.email", "test@example.com")
	run("commit", "--allow-empty", "-m", "init")

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

// TestPerRepoGitPollerEmitsRepoTagged verifies the M2 fix: the git poller runs
// per registered repo, so a git change in a NON-active repo (web) produces a
// repo-tagged git.updated SSE event. Previously only the active repo was polled.
func TestPerRepoGitPollerEmitsRepoTagged(t *testing.T) {
	api := gitInitRepo(t, "api")
	web := gitInitRepo(t, "web")
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
	srv.gitPollInterval = 60 * time.Millisecond
	stop := srv.StartLiveSync()
	defer stop()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	lines := make(chan string, 64)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()
	if !waitFor(lines, "connected", 3*time.Second) {
		t.Fatal("never received SSE connect")
	}
	// The pollers take their baseline snapshots synchronously in
	// startRepoGitPoller (before the goroutine loop), so StartLiveSync returning
	// guarantees both repos' `prev` is anchored to current HEAD. No sleep needed:
	// the commit below is observed as a change, never captured as the baseline.

	// Make a git change in the NON-active repo (web): a new empty commit moves HEAD.
	webRoot := filepath.Dir(web.Root())
	cmd := exec.Command("git", "-C", webRoot, "commit", "--allow-empty", "-m", "two")
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit in web: %v\n%s", err, out)
	}

	// Expect a git.updated data line tagged "repo":"web" (the active repo api
	// is unchanged, so it must not be the source of the event).
	var seen []string
	deadline := time.After(5 * time.Second)
	for {
		select {
		case l := <-lines:
			seen = append(seen, l)
			if strings.Contains(l, `"repo":"web"`) && strings.Contains(l, `"git"`) {
				return
			}
		case <-deadline:
			t.Fatalf("no repo-tagged git.updated for non-active repo web; saw %d lines:\n%s",
				len(seen), strings.Join(seen, "\n"))
		}
	}
}

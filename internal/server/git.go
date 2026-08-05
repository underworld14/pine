package server

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"time"

	"github.com/underworld14/pine/internal/gitx"
	"github.com/underworld14/pine/internal/store"
)

const (
	gitCommitLimit = 10
	gitTimeout     = 3 * time.Second
	gitPollEvery   = 5 * time.Second
	fileSuggestCap = 50
)

// initGit creates the git client and takes an initial snapshot so /api/git and
// the hydration snapshot have data immediately.
func (srv *Server) initGit() {
	client := gitx.New(filepath.Dir(srv.activeStore().Root()))
	srv.gitMu.Lock()
	srv.git = client
	srv.gitMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	srv.setGitStatus(client.Snapshot(ctx, gitCommitLimit))
}

// gitClient returns the current git client under the lock, so pollers and
// SetActiveRepo can't race on srv.git being swapped.
func (srv *Server) gitClient() gitx.Client {
	srv.gitMu.RLock()
	defer srv.gitMu.RUnlock()
	return srv.git
}

func (srv *Server) setGitStatus(s gitx.Status) {
	srv.gitMu.Lock()
	srv.gitStatus = s
	srv.gitMu.Unlock()
}

func (srv *Server) gitSnapshot() gitx.Status {
	srv.gitMu.RLock()
	defer srv.gitMu.RUnlock()
	return srv.gitStatus
}

// startRepoGitPoller refreshes one repo's git state off the request path and
// broadcasts a repo-tagged git.updated only when that repo's snapshot actually
// changes. The poller for the currently-active repo also updates srv.gitStatus
// (used by /api/git and the hydration snapshot) and kicks the cross-branch
// overlay, so the active repo's git stays live regardless of which repo is
// active. Polling every registered repo (not just the active one) means a
// switch to a previously-inactive repo shows its latest git state immediately.
func (srv *Server) startRepoGitPoller(done chan struct{}, alias string, st *store.Store) {
	client := gitx.New(filepath.Dir(st.Root()))
	interval := srv.gitPollInterval
	if interval <= 0 {
		interval = gitPollEvery
	}
	// Take the baseline snapshot synchronously before the loop starts. This
	// guarantees that once StartLiveSync returns, every poller's `prev` is
	// already anchored to the repo's current HEAD — so a change committed
	// immediately after serve starts is observed as a change (not captured as
	// the baseline by a racing goroutine). Snapshots are fast (a bounded git
	// log); doing them sequentially at startup is cheaper than the flaky
	// "sleep before the test commit" alternative.
	ctx0, cancel0 := context.WithTimeout(context.Background(), gitTimeout)
	prev := hashStatus(client.Snapshot(ctx0, gitCommitLimit))
	cancel0()
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
				s := client.Snapshot(ctx, gitCommitLimit)
				cancel()
				h := hashStatus(s)
				if h == prev {
					continue
				}
				prev = h
				if srv.isActiveStore(st) {
					srv.setGitStatus(s)
					srv.kickCrossBranch() // HEAD/branch may have moved; refresh the overlay
				}
				srv.emitRepo(alias, "git.updated", fsOrigin(), map[string]any{"git": s})
			}
		}
	}()
}

func hashStatus(s gitx.Status) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func (srv *Server) handleGit(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, srv.gitFor(r))
}

// gitFor returns the git status for the request's store: the cached snapshot for
// the active store, or a fresh snapshot for an alias-routed non-active store.
func (srv *Server) gitFor(r *http.Request) gitx.Status {
	st := srv.storeOf(r)
	if srv.isActiveStore(st) {
		return srv.gitSnapshot()
	}
	ctx, cancel := context.WithTimeout(r.Context(), gitTimeout)
	defer cancel()
	return gitx.New(filepath.Dir(st.Root())).Snapshot(ctx, gitCommitLimit)
}

// handleFiles suggests tracked file and directory paths matching q (for the
// "@" related-files autocomplete in the editor).
func (srv *Server) handleFiles(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	ctx, cancel := context.WithTimeout(r.Context(), gitTimeout)
	defer cancel()
	client := srv.gitClient()
	if st := srv.storeOf(r); !srv.isActiveStore(st) {
		client = gitx.New(filepath.Dir(st.Root()))
	}
	items := suggestFileItems(client.Files(ctx), q, fileSuggestCap)
	files := make([]string, 0, len(items))
	for _, it := range items {
		if it.Kind == "file" {
			files = append(files, it.Path)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "files": files})
}

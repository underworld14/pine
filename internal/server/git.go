package server

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/underworld14/pine/internal/gitx"
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
	srv.git = gitx.New(filepath.Dir(srv.store.Root()))
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	srv.setGitStatus(srv.git.Snapshot(ctx, gitCommitLimit))
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

// startGitPoller refreshes git state off the request path and broadcasts
// git.updated only when the snapshot actually changes.
func (srv *Server) startGitPoller(done chan struct{}) {
	go func() {
		ticker := time.NewTicker(gitPollEvery)
		defer ticker.Stop()
		prev := hashStatus(srv.gitSnapshot())
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
				s := srv.git.Snapshot(ctx, gitCommitLimit)
				cancel()
				if h := hashStatus(s); h != prev {
					prev = h
					srv.setGitStatus(s)
					srv.emit("git.updated", fsOrigin(), map[string]any{"git": s})
				}
			}
		}
	}()
}

func hashStatus(s gitx.Status) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func (srv *Server) handleGit(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, srv.gitSnapshot())
}

// handleFiles suggests tracked file paths matching q (for the "Related Files"
// autocomplete that replaces the PRD's smart-file-detection magic).
func (srv *Server) handleFiles(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(r.URL.Query().Get("q"))
	ctx, cancel := context.WithTimeout(r.Context(), gitTimeout)
	defer cancel()
	out := []string{}
	for _, f := range srv.git.Files(ctx) {
		if q == "" || strings.Contains(strings.ToLower(f), q) {
			out = append(out, f)
			if len(out) >= fileSuggestCap {
				break
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": out})
}

package server

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/izzadev/pine/internal/attach"
	"github.com/izzadev/pine/internal/store"
	"github.com/izzadev/pine/internal/view"
)

const maxUploadBytes = 512 << 20 // 512 MB (covers video)

// attachResult is one file's outcome in the upload response.
type attachResult struct {
	Name          string `json:"name"`
	Path          string `json:"path"`
	Markdown      string `json:"markdown"`
	URL           string `json:"url"`
	Mime          string `json:"mime"`
	Kind          string `json:"kind"`
	OriginalBytes int64  `json:"originalBytes"`
	FinalBytes    int64  `json:"finalBytes"`
	Width         int    `json:"width,omitempty"`
	Height        int    `json:"height,omitempty"`
	Optimized     bool   `json:"optimized"`
	Deduplicated  bool   `json:"deduplicated"`
	Warning       string `json:"warning,omitempty"`
	Error         string `json:"error,omitempty"`
}

func (srv *Server) handleUploadAttachments(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := srv.store.Get(id); err != nil {
		writeErr(w, err)
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeErr(w, badRequest("could not parse upload: "+err.Error()))
		return
	}
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		writeErr(w, badRequest("no files in the 'files' field"))
		return
	}

	cfg := attach.FromConfig(srv.store.Config().Attachments)
	existing := map[string]bool{}
	for _, a := range srv.store.Attachments(id) {
		existing[a.Name] = true
	}

	var results []attachResult
	var warnings []string
	anyOK := false

	for _, fh := range files {
		f, err := fh.Open()
		if err != nil {
			results = append(results, attachResult{Name: fh.Filename, Error: "could not open upload"})
			continue
		}
		data, err := io.ReadAll(io.LimitReader(f, maxUploadBytes))
		f.Close()
		if err != nil {
			results = append(results, attachResult{Name: fh.Filename, Error: "could not read upload"})
			continue
		}
		p, err := attach.Process(fh.Filename, data, cfg)
		if err != nil {
			results = append(results, attachResult{Name: fh.Filename, Error: err.Error()})
			continue
		}
		dedup := existing[p.FileName]
		if !dedup {
			if _, err := srv.store.WriteAttachment(id, p.FileName, p.Data); err != nil {
				results = append(results, attachResult{Name: p.FileName, Error: err.Error()})
				continue
			}
			existing[p.FileName] = true
		}
		anyOK = true
		results = append(results, buildAttachResult(id, p, dedup))
		if p.Warning != "" {
			warnings = append(warnings, p.Warning)
		}
	}

	if anyOK {
		if t, err := srv.store.Get(id); err == nil {
			srv.emit("ticket.updated", apiOrigin(r.URL.Query().Get("opId")), map[string]any{
				"ticket": view.Build(srv.store, srv.store.Graph(), t, true),
			})
		}
	}

	status := http.StatusCreated
	if !anyOK {
		status = http.StatusUnprocessableEntity
	}
	writeJSON(w, status, map[string]any{"attachments": results, "warnings": warnings})
}

func buildAttachResult(id string, p attach.Processed, dedup bool) attachResult {
	alt := strings.TrimSuffix(p.FileName, filepath.Ext(p.FileName))
	return attachResult{
		Name:          p.FileName,
		Path:          "attachments/" + id + "/" + p.FileName,
		Markdown:      "![" + alt + "](../attachments/" + id + "/" + p.FileName + ")",
		URL:           "/attachments/" + id + "/" + p.FileName,
		Mime:          p.Mime,
		Kind:          p.Kind,
		OriginalBytes: p.OriginalBytes,
		FinalBytes:    p.FinalBytes,
		Width:         p.Width,
		Height:        p.Height,
		Optimized:     p.Optimized,
		Deduplicated:  dedup,
		Warning:       p.Warning,
	}
}

func (srv *Server) handleDeleteAttachment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	name := chi.URLParam(r, "name")
	if err := srv.store.DeleteAttachment(id, name); err != nil {
		writeErr(w, badRequest(err.Error()))
		return
	}
	if t, err := srv.store.Get(id); err == nil {
		srv.emit("ticket.updated", apiOrigin(r.URL.Query().Get("opId")), map[string]any{
			"ticket": view.Build(srv.store, srv.store.Graph(), t, true),
		})
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleServeAttachment serves an attachment file. The ticket id and filename are
// validated and confined to the attachments directory by the store.
func (srv *Server) handleServeAttachment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	name := chi.URLParam(r, "name")
	path, err := srv.store.AttachmentFilePath(id, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	mime, _ := store.MimeAndKind(name)
	w.Header().Set("Content-Type", mime)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, path)
}

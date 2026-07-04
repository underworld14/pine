package store

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/underworld14/pine/internal/ticket"
)

// AttachmentInfo describes one file in a ticket's attachments directory.
type AttachmentInfo struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Mime string `json:"mime"`
	Kind string `json:"kind"` // image | video | other
	URL  string `json:"url"`  // /attachments/<id>/<name>
}

func (s *Store) attachmentsRoot() string { return filepath.Join(s.root, dirAttachments) }
func (s *Store) attachmentDir(id string) string {
	return filepath.Join(s.attachmentsRoot(), id)
}

// Attachments lists a ticket's attachments (dot-files and temp files excluded).
func (s *Store) Attachments(id string) []AttachmentInfo {
	entries, err := os.ReadDir(s.attachmentDir(id))
	if err != nil {
		return nil
	}
	var out []AttachmentInfo
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		mime, kind := MimeAndKind(e.Name())
		out = append(out, AttachmentInfo{
			Name: e.Name(),
			Size: info.Size(),
			Mime: mime,
			Kind: kind,
			URL:  "/attachments/" + id + "/" + e.Name(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// WriteAttachment writes bytes to a ticket's attachment directory atomically.
// The name is sanitized; callers that optimize the bytes pass the final name.
func (s *Store) WriteAttachment(id, name string, data []byte) (AttachmentInfo, error) {
	if !ticket.ValidID(id) {
		return AttachmentInfo{}, errors.New("invalid ticket id")
	}
	safe, err := sanitizeName(name)
	if err != nil {
		return AttachmentInfo{}, err
	}
	if err := atomicWrite(filepath.Join(s.attachmentDir(id), safe), data); err != nil {
		return AttachmentInfo{}, err
	}
	mime, kind := MimeAndKind(safe)
	return AttachmentInfo{
		Name: safe,
		Size: int64(len(data)),
		Mime: mime,
		Kind: kind,
		URL:  "/attachments/" + id + "/" + safe,
	}, nil
}

// DeleteAttachment removes one file from a ticket's attachment directory.
func (s *Store) DeleteAttachment(id, name string) error {
	safe, err := sanitizeName(name)
	if err != nil {
		return err
	}
	err = os.Remove(filepath.Join(s.attachmentDir(id), safe))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// AttachmentFilePath resolves and validates a path for serving an attachment,
// guaranteeing the result stays within the attachments root (no traversal).
func (s *Store) AttachmentFilePath(id, name string) (string, error) {
	if !ticket.ValidID(id) {
		return "", errors.New("invalid ticket id")
	}
	safe, err := sanitizeName(name)
	if err != nil {
		return "", err
	}
	full := filepath.Join(s.attachmentDir(id), safe)
	rel, err := filepath.Rel(s.attachmentsRoot(), full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes attachments directory")
	}
	return full, nil
}

// AttachmentDirs returns the ticket IDs that have an attachments directory,
// used by doctor to find orphans.
func (s *Store) AttachmentDirs() []string {
	entries, err := os.ReadDir(s.attachmentsRoot())
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// sanitizeName reduces a client filename to a safe basename or rejects it.
func sanitizeName(name string) (string, error) {
	base := filepath.Base(strings.TrimSpace(name))
	if base == "" || base == "." || base == ".." {
		return "", errors.New("invalid filename")
	}
	if strings.ContainsAny(base, `/\`) || strings.Contains(base, "..") {
		return "", errors.New("invalid filename")
	}
	if strings.HasPrefix(base, ".") {
		return "", errors.New("invalid filename")
	}
	return base, nil
}

// MimeAndKind maps a filename extension to a MIME type and a coarse kind.
func MimeAndKind(name string) (mime, kind string) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png":
		return "image/png", "image"
	case ".jpg", ".jpeg":
		return "image/jpeg", "image"
	case ".gif":
		return "image/gif", "image"
	case ".webp":
		return "image/webp", "image"
	case ".mp4":
		return "video/mp4", "video"
	case ".mov":
		return "video/quicktime", "video"
	default:
		return "application/octet-stream", "other"
	}
}

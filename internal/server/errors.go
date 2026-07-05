package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/underworld14/pine/internal/store"
)

// httpError carries an explicit status and machine code for the error body.
type httpError struct {
	status int
	code   string
	msg    string
}

func (e httpError) Error() string { return e.msg }

func badRequest(msg string) error { return httpError{http.StatusBadRequest, "bad_request", msg} }
func unprocessable(msg string) error {
	return httpError{http.StatusUnprocessableEntity, "validation_failed", msg}
}

// readOnlyBranch rejects a write to a ticket that lives on another git branch.
func readOnlyBranch(id, branch string) error {
	return httpError{
		http.StatusConflict, "off_branch",
		id + " is read-only; it lives on branch \"" + branch + "\". Check out that branch to edit it.",
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeErr maps store and http errors to a JSON error body with a stable code.
func writeErr(w http.ResponseWriter, err error) {
	status, code := http.StatusInternalServerError, "internal"
	var he httpError
	switch {
	case errors.As(err, &he):
		status, code = he.status, he.code
	case errors.Is(err, store.ErrNotFound):
		status, code = http.StatusNotFound, "not_found"
	case errors.Is(err, store.ErrUnknownType):
		status, code = http.StatusUnprocessableEntity, "unknown_type"
	case errors.Is(err, store.ErrDegraded):
		status, code = http.StatusConflict, "degraded"
	}
	writeJSON(w, status, map[string]any{
		"error": map[string]any{"code": code, "message": err.Error()},
	})
}

func readBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(io.LimitReader(r.Body, 8<<20))
}

func decodeJSON(r *http.Request, v any) error {
	body, err := readBody(r)
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, v)
}

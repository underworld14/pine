package server

import (
	"strings"
	"testing"
)

func TestHandleContext(t *testing.T) {
	ts := newTestServer(t)
	do(t, "POST", ts.URL+"/api/tickets", `{"type":"bug","title":"Context Ticket","priority":"critical"}`, nil)

	resp, body := do(t, "GET", ts.URL+"/api/context", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/markdown; charset=utf-8" {
		t.Errorf("content-type = %q", ct)
	}
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("missing nosniff header")
	}
	if !strings.Contains(body, "# Project Context: test") {
		t.Errorf("body missing project header: %s", body)
	}
	if !strings.Contains(body, "BUG-001") {
		t.Errorf("body should mention the critical ticket: %s", body)
	}
}

func TestHandlePrompt(t *testing.T) {
	ts := newTestServer(t)
	do(t, "POST", ts.URL+"/api/tickets", `{"type":"bug","title":"Prompt Ticket","priority":"high"}`, nil)

	resp, body := do(t, "GET", ts.URL+"/api/tickets/BUG-001/prompt", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/markdown; charset=utf-8" {
		t.Errorf("content-type = %q", ct)
	}
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("missing nosniff header")
	}
	if !strings.Contains(body, "# Fix Request: BUG-001") {
		t.Errorf("body missing fix request header: %s", body)
	}
	if !strings.Contains(body, "Prompt Ticket") {
		t.Errorf("body should include the ticket title: %s", body)
	}
}

func TestHandlePromptUnknownTicket404(t *testing.T) {
	ts := newTestServer(t)
	resp, body := do(t, "GET", ts.URL+"/api/tickets/BUG-999/prompt", "", nil)
	if resp.StatusCode != 404 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "not_found") {
		t.Errorf("expected not_found code: %s", body)
	}
}

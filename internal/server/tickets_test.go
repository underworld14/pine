package server

import (
	"strings"
	"testing"
)

func TestHandleListTickets(t *testing.T) {
	ts := newTestServer(t)
	do(t, "POST", ts.URL+"/api/tickets", `{"type":"bug","title":"Login broken","priority":"high","labels":["ui"]}`, nil)
	do(t, "POST", ts.URL+"/api/tickets", `{"type":"feature","title":"Dark mode","status":"doing"}`, nil)

	resp, body := do(t, "GET", ts.URL+"/api/tickets", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	tickets := snapshotTickets(t, body)
	if len(tickets) != 2 {
		t.Fatalf("expected 2 tickets, got %d: %s", len(tickets), body)
	}
}

func TestHandleListTicketsFilters(t *testing.T) {
	ts := newTestServer(t)
	do(t, "POST", ts.URL+"/api/tickets", `{"type":"bug","title":"Login broken","priority":"high","labels":["ui"]}`, nil)
	do(t, "POST", ts.URL+"/api/tickets", `{"type":"feature","title":"Dark mode","status":"doing"}`, nil)

	// Filter by status.
	_, body := do(t, "GET", ts.URL+"/api/tickets?status=doing", "", nil)
	tickets := snapshotTickets(t, body)
	if len(tickets) != 1 || tickets[0].ID != "FEAT-001" {
		t.Fatalf("status filter: %+v", tickets)
	}

	// Filter by type.
	_, body = do(t, "GET", ts.URL+"/api/tickets?type=bug", "", nil)
	tickets = snapshotTickets(t, body)
	if len(tickets) != 1 || tickets[0].ID != "BUG-001" {
		t.Fatalf("type filter: %+v", tickets)
	}

	// Filter by label.
	_, body = do(t, "GET", ts.URL+"/api/tickets?label=ui", "", nil)
	tickets = snapshotTickets(t, body)
	if len(tickets) != 1 || tickets[0].ID != "BUG-001" {
		t.Fatalf("label filter: %+v", tickets)
	}

	// Filter matching nothing.
	_, body = do(t, "GET", ts.URL+"/api/tickets?status=nope", "", nil)
	tickets = snapshotTickets(t, body)
	if len(tickets) != 0 {
		t.Fatalf("expected no matches: %+v", tickets)
	}
}

func TestHandleListTicketsParentFilter(t *testing.T) {
	ts := newTestServer(t)
	_, epicBody := do(t, "POST", ts.URL+"/api/tickets", `{"type":"epic","title":"Epic One"}`, nil)
	if !strings.Contains(epicBody, "EPIC-001") {
		t.Fatalf("expected EPIC-001: %s", epicBody)
	}
	do(t, "POST", ts.URL+"/api/tickets", `{"type":"bug","title":"Child","parent":"EPIC-001"}`, nil)
	do(t, "POST", ts.URL+"/api/tickets", `{"type":"bug","title":"Orphan"}`, nil)

	_, body := do(t, "GET", ts.URL+"/api/tickets?parent=EPIC-001", "", nil)
	tickets := snapshotTickets(t, body)
	if len(tickets) != 1 || tickets[0].ID != "BUG-001" {
		t.Fatalf("parent filter: %+v", tickets)
	}
}

func TestHandleCreateTicketMalformedJSON400(t *testing.T) {
	ts := newTestServer(t)
	resp, body := do(t, "POST", ts.URL+"/api/tickets", `{not json`, nil)
	if resp.StatusCode != 400 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "bad_request") {
		t.Errorf("expected bad_request code: %s", body)
	}
}

func TestHandleUpdateTicketAllFields(t *testing.T) {
	ts := newTestServer(t)
	do(t, "POST", ts.URL+"/api/tickets", `{"type":"bug","title":"Original"}`, nil)
	do(t, "POST", ts.URL+"/api/tickets", `{"type":"bug","title":"Dep"}`, nil)

	patch := `{"title":"Updated","status":"doing","priority":"low","labels":["x","y"],"deps":["BUG-002"],"body":"new body"}`
	resp, body := do(t, "PUT", ts.URL+"/api/tickets/BUG-001", patch, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"title":"Updated"`) || !strings.Contains(body, `"status":"doing"`) ||
		!strings.Contains(body, `"priority":"low"`) || !strings.Contains(body, "new body") {
		t.Errorf("update did not apply all fields: %s", body)
	}
}

func TestHandleUpdateTicketMalformedJSON400(t *testing.T) {
	ts := newTestServer(t)
	do(t, "POST", ts.URL+"/api/tickets", `{"type":"bug","title":"x"}`, nil)
	resp, body := do(t, "PATCH", ts.URL+"/api/tickets/BUG-001", `{bad`, nil)
	if resp.StatusCode != 400 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "bad_request") {
		t.Errorf("expected bad_request code: %s", body)
	}
}

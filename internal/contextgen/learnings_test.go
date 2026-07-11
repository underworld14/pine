package contextgen

import (
	"strings"
	"testing"
	"time"

	"github.com/underworld14/pine/internal/store"
)

// Regression tests for the scope-isolation fix in SelectLearnings: a
// ticket-scoped learning must never surface for a different ticket, for
// `pine context` (no ticket), or via a supersede chain that resolves to a
// tip scoped to yet another ticket.

func TestSelectLearningsNoCrossTicketLeak(t *testing.T) {
	s := scaffold(t)
	tkA, err := s.Create(store.CreateReq{Type: "bug", Title: "Auth bug A", Labels: []string{"auth"}})
	if err != nil {
		t.Fatal(err)
	}
	tkB, err := s.Create(store.CreateReq{Type: "bug", Title: "Auth bug B", Labels: []string{"auth"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateLearning(store.CreateLearningReq{
		Text: "UNIQUE_TICKET_A_ONLY insight", Scope: "ticket", Ticket: tkA.ID, Tags: []string{"auth"},
	}); err != nil {
		t.Fatal(err)
	}

	// A different ticket sharing the same label/tag must not see it, even
	// though the old tag-overlap admission path would have matched it.
	outB, err := Prompt(s, fakeGit(), tkB.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(outB, "UNIQUE_TICKET_A_ONLY") {
		t.Fatalf("ticket-scoped learning for %s leaked into prompt for %s:\n%s", tkA.ID, tkB.ID, outB)
	}

	// pine context (no ticket at all) must not see it either.
	md := Context(s, fakeGit(), time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC))
	if strings.Contains(md, "UNIQUE_TICKET_A_ONLY") {
		t.Fatalf("ticket-scoped learning leaked into pine context:\n%s", md)
	}

	// Its own ticket must still see it.
	outA, err := Prompt(s, fakeGit(), tkA.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(outA, "UNIQUE_TICKET_A_ONLY") {
		t.Fatalf("ticket-scoped learning missing from its own ticket's prompt:\n%s", outA)
	}
}

func TestSelectLearningsTipMustMatchOwnScope(t *testing.T) {
	s := scaffold(t)
	tkA, err := s.Create(store.CreateReq{Type: "bug", Title: "Ticket A"})
	if err != nil {
		t.Fatal(err)
	}
	tkB, err := s.Create(store.CreateReq{Type: "bug", Title: "Ticket B"})
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.CreateLearning(store.CreateLearningReq{
		Text: "original rule for A", Scope: "ticket", Ticket: tkA.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateLearning(store.CreateLearningReq{
		Text: "UNIQUE_TICKET_B_REPLACEMENT rule", Scope: "ticket", Ticket: tkB.ID, Supersedes: a.ID,
	}); err != nil {
		t.Fatal(err)
	}
	out, err := Prompt(s, fakeGit(), tkA.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "UNIQUE_TICKET_B_REPLACEMENT") {
		t.Fatalf("learning scoped to ticket B leaked into ticket A's prompt via tip-substitution of a superseded ticket-A learning:\n%s", out)
	}
}

package view

import (
	"testing"
	"time"

	"github.com/underworld14/pine/internal/ticket"
)

func TestBuildOffBranchAcceptance(t *testing.T) {
	tk := &ticket.Ticket{
		ID: "BUG-7f3k2a", Title: "x", Status: "todo",
		Created: time.Now(), Updated: time.Now(),
		Body: "# Acceptance Criteria\n- [x] a\n- [ ] b\n",
	}
	v := BuildOffBranch(tk, "feature", false)
	if v.Acceptance == nil || v.Acceptance.Done != 1 || v.Acceptance.Total != 2 {
		t.Fatalf("acceptance = %+v, want 1/2", v.Acceptance)
	}

	tk.Body = "# Description\nno criteria\n"
	if v := BuildOffBranch(tk, "feature", false); v.Acceptance != nil {
		t.Errorf("no AC section should omit acceptance, got %+v", v.Acceptance)
	}
}

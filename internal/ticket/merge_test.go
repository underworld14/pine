package ticket

import (
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func mt(title, status string, labels, deps []string, body string, updated time.Time) *Ticket {
	return &Ticket{ID: "BUG-001", Title: title, Status: status, Labels: labels, Deps: deps, Body: body, Updated: updated}
}

func TestMerge3OneSidedScalar(t *testing.T) {
	base := mt("T", "todo", nil, nil, "b", time.Time{})
	ours := mt("T", "doing", nil, nil, "b", time.Now())
	theirs := mt("T", "todo", nil, nil, "b", time.Time{})
	m, conflict := Merge3(base, ours, theirs)
	if conflict {
		t.Error("one-sided status change should not conflict")
	}
	if m.Status != "doing" {
		t.Errorf("status = %q, want doing", m.Status)
	}
}

func TestMerge3SetUnionAndDeletion(t *testing.T) {
	base := mt("T", "todo", []string{"a"}, []string{"X", "Y"}, "b", time.Time{})
	ours := mt("T", "todo", []string{"a", "b"}, []string{"X"}, "b", time.Time{})        // added label b, deleted dep Y
	theirs := mt("T", "todo", []string{"a", "c"}, []string{"X", "Y"}, "b", time.Time{}) // added label c
	m, conflict := Merge3(base, ours, theirs)
	if conflict {
		t.Error("set merge should not conflict")
	}
	if got := strings.Join(m.Labels, ","); got != "a,b,c" {
		t.Errorf("labels = %q, want a,b,c", got)
	}
	if got := strings.Join(m.Deps, ","); got != "X" {
		t.Errorf("deps = %q, want X (Y deleted by ours)", got)
	}
}

func TestMerge3ScalarBothChangedNewerWins(t *testing.T) {
	older := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	base := mt("Base", "todo", nil, nil, "b", time.Time{})
	ours := mt("Ours", "todo", nil, nil, "b", older)
	theirs := mt("Theirs", "todo", nil, nil, "b", newer)
	m, conflict := Merge3(base, ours, theirs)
	if !conflict {
		t.Error("both-sided title change should flag a conflict")
	}
	if m.Title != "Theirs" {
		t.Errorf("title = %q, want Theirs (newer Updated wins)", m.Title)
	}
	if !m.Updated.Equal(newer) {
		t.Errorf("updated = %v, want %v", m.Updated, newer)
	}
}

func TestMerge3BodyOneSided(t *testing.T) {
	base := mt("T", "todo", nil, nil, "original\n", time.Time{})
	ours := mt("T", "todo", nil, nil, "original\n", time.Time{})
	theirs := mt("T", "todo", nil, nil, "updated by them\n", time.Time{})
	m, conflict := Merge3(base, ours, theirs)
	if conflict {
		t.Error("one-sided body change should not conflict")
	}
	if m.Body != "updated by them\n" {
		t.Errorf("body = %q", m.Body)
	}
}

func TestMerge3BodyBothChangedConflicts(t *testing.T) {
	base := mt("T", "todo", nil, nil, "original\n", time.Time{})
	ours := mt("T", "todo", nil, nil, "our version\n", time.Time{})
	theirs := mt("T", "todo", nil, nil, "their version\n", time.Time{})
	m, conflict := Merge3(base, ours, theirs)
	if !conflict {
		t.Fatal("both-sided body change should conflict")
	}
	if !strings.Contains(m.Body, "<<<<<<< ours") || !strings.Contains(m.Body, "our version") ||
		!strings.Contains(m.Body, "=======") || !strings.Contains(m.Body, "their version") ||
		!strings.Contains(m.Body, ">>>>>>> theirs") {
		t.Errorf("expected conflict markers, got:\n%s", m.Body)
	}
}

func TestMerge3AddAddNoBase(t *testing.T) {
	older := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	ours := mt("Ours", "todo", nil, nil, "b", older)
	theirs := mt("Theirs", "doing", nil, nil, "b", newer)
	m, conflict := Merge3(nil, ours, theirs)
	if !conflict {
		t.Error("add/add with differing titles should conflict")
	}
	if m.Title != "Theirs" || m.Status != "doing" {
		t.Errorf("newer side should win: title=%q status=%q", m.Title, m.Status)
	}
}

func TestMerge3ExtraKeyDeletionHonored(t *testing.T) {
	gh := ExtraField{Key: "github", Node: &yaml.Node{Kind: yaml.ScalarNode, Value: "https://x/1"}}
	base := &Ticket{ID: "BUG-001", Title: "T", Body: "b", Extra: []ExtraField{gh}}
	ours := &Ticket{ID: "BUG-001", Title: "T", Body: "b", Extra: []ExtraField{gh}} // kept
	theirs := &Ticket{ID: "BUG-001", Title: "T", Body: "b", Extra: nil}            // deleted github
	m, _ := Merge3(base, ours, theirs)
	for _, e := range m.Extra {
		if e.Key == "github" {
			t.Errorf("a base Extra key deleted on one side must not resurrect, got %v", m.Extra)
		}
	}
	// A newly-added key (not in base) is still kept.
	added := ExtraField{Key: "reviewed", Node: &yaml.Node{Kind: yaml.ScalarNode, Value: "true"}}
	ours2 := &Ticket{ID: "BUG-001", Body: "b", Extra: []ExtraField{added}}
	theirs2 := &Ticket{ID: "BUG-001", Body: "b"}
	m2, _ := Merge3(base, ours2, theirs2)
	found := false
	for _, e := range m2.Extra {
		if e.Key == "reviewed" {
			found = true
		}
	}
	if !found {
		t.Errorf("a newly-added Extra key must be kept, got %v", m2.Extra)
	}
}

package tui

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/huh"
)

func withRunForm(t *testing.T, fn func(*huh.Form) error) {
	t.Helper()
	prev := runForm
	runForm = fn
	t.Cleanup(func() { runForm = prev })
}

func TestConfirmIOAcceptsDefault(t *testing.T) {
	withRunForm(t, func(*huh.Form) error { return nil })
	ok, err := ConfirmIO("Continue?", "desc", true, strings.NewReader(""), io.Discard)
	if err != nil || !ok {
		t.Fatalf("got ok=%v err=%v", ok, err)
	}
}

func TestConfirmIOCancel(t *testing.T) {
	withRunForm(t, func(*huh.Form) error { return huh.ErrUserAborted })
	ok, err := Confirm("Abort?", "", false)
	if ok || !errors.Is(err, ErrCancelled) {
		t.Fatalf("got ok=%v err=%v", ok, err)
	}
}

func TestConfirmIOOtherError(t *testing.T) {
	want := errors.New("boom")
	withRunForm(t, func(*huh.Form) error { return want })
	ok, err := ConfirmIO("x", "", false, nil, nil)
	if ok || !errors.Is(err, want) {
		t.Fatalf("got ok=%v err=%v", ok, err)
	}
}

func TestConfirmDeleteIO(t *testing.T) {
	withRunForm(t, func(*huh.Form) error { return nil })
	ok, err := ConfirmDeleteIO("LRN-001", strings.NewReader(""), bytes.NewBuffer(nil))
	if err != nil || ok {
		// defaultYes is false for delete
		t.Fatalf("got ok=%v err=%v (want false,nil)", ok, err)
	}
	ok, err = ConfirmDelete("LRN-002")
	if err != nil || ok {
		t.Fatalf("ConfirmDelete: ok=%v err=%v", ok, err)
	}
}

func TestMultiSelectIOSelectedDefaults(t *testing.T) {
	withRunForm(t, func(*huh.Form) error { return nil })
	got, err := MultiSelectIO("Pick agents", []Choice{
		{Key: "agents", Label: "AGENTS.md", Description: "Codex/Cursor", Selected: true},
		{Key: "claude", Label: "CLAUDE.md", Selected: false},
	}, strings.NewReader(""), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "agents" {
		t.Fatalf("got %#v", got)
	}
}

func TestMultiSelectCancelAndError(t *testing.T) {
	withRunForm(t, func(*huh.Form) error { return huh.ErrUserAborted })
	_, err := MultiSelect("x", []Choice{{Key: "a", Label: "A", Selected: true}})
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("err=%v", err)
	}

	want := errors.New("nope")
	withRunForm(t, func(*huh.Form) error { return want })
	_, err = MultiSelectIO("x", []Choice{{Key: "a", Label: "A"}}, nil, nil)
	if !errors.Is(err, want) {
		t.Fatalf("err=%v", err)
	}
}

func TestMultiSelectAllowEmpty(t *testing.T) {
	withRunForm(t, func(*huh.Form) error { return nil })
	got, err := MultiSelectAllowEmpty("Pick none", []Choice{
		{Key: "tickets", Label: "Tickets", Description: ".pine/tickets/", Selected: false},
		{Key: "attachments", Label: "Attachments", Selected: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("allow-empty should keep zero selection, got %#v", got)
	}
}

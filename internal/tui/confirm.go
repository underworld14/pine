package tui

import (
	"errors"
	"fmt"
	"io"

	"github.com/charmbracelet/huh"
)

// ErrCancelled is returned when the user aborts an interactive prompt.
var ErrCancelled = errors.New("cancelled")

// runForm executes a huh form. Overridable in tests.
var runForm = func(form *huh.Form) error { return form.Run() }

// Confirm runs a Yes/No prompt. defaultYes selects the affirmative button initially.
// Returns (false, ErrCancelled) when the user aborts (Esc / Ctrl+C).
func Confirm(title, description string, defaultYes bool) (bool, error) {
	return ConfirmIO(title, description, defaultYes, nil, nil)
}

// ConfirmIO is Confirm with optional input/output overrides (nil → stdin/stdout).
func ConfirmIO(title, description string, defaultYes bool, in io.Reader, out io.Writer) (bool, error) {
	value := defaultYes
	field := huh.NewConfirm().
		Title(title).
		Description(description).
		Affirmative("Yes").
		Negative("No").
		Value(&value)

	form := huh.NewForm(huh.NewGroup(field)).WithShowHelp(true)
	if in != nil {
		form = form.WithInput(in)
	}
	if out != nil {
		form = form.WithOutput(out)
	}

	if err := runForm(form); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return false, ErrCancelled
		}
		return false, err
	}
	return value, nil
}

// ConfirmDelete is a convenience confirm for destructive actions (defaults to No).
func ConfirmDelete(summary string) (bool, error) {
	return ConfirmDeleteIO(summary, nil, nil)
}

// ConfirmDeleteIO is ConfirmDelete with optional input/output overrides.
func ConfirmDeleteIO(summary string, in io.Reader, out io.Writer) (bool, error) {
	return ConfirmIO(fmt.Sprintf("Delete %s?", summary), "", false, in, out)
}

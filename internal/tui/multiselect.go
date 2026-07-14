package tui

import (
	"errors"
	"fmt"
	"io"

	"github.com/charmbracelet/huh"
)

// Choice is one option in a multi-select prompt.
type Choice struct {
	Key         string
	Label       string
	Description string
	Selected    bool
}

// MultiSelect runs a checklist prompt and returns the keys of selected choices.
// At least one choice must be selected. Returns ErrCancelled when the user aborts.
func MultiSelect(title string, choices []Choice) ([]string, error) {
	return MultiSelectIO(title, choices, nil, nil)
}

// MultiSelectAllowEmpty is MultiSelect without the "at least one" requirement.
func MultiSelectAllowEmpty(title string, choices []Choice) ([]string, error) {
	return multiSelectIO(title, choices, nil, nil, true)
}

// MultiSelectIO is MultiSelect with optional input/output overrides.
func MultiSelectIO(title string, choices []Choice, in io.Reader, out io.Writer) ([]string, error) {
	return multiSelectIO(title, choices, in, out, false)
}

func multiSelectIO(title string, choices []Choice, in io.Reader, out io.Writer, allowEmpty bool) ([]string, error) {
	options := make([]huh.Option[string], 0, len(choices))
	selected := make([]string, 0, len(choices))
	for _, c := range choices {
		label := c.Label
		if c.Description != "" {
			label = fmt.Sprintf("%-12s (%s)", c.Label, c.Description)
		}
		opt := huh.NewOption(label, c.Key)
		if c.Selected {
			opt = opt.Selected(true)
			selected = append(selected, c.Key)
		}
		options = append(options, opt)
	}

	field := huh.NewMultiSelect[string]().
		Title(title).
		Options(options...).
		Filterable(false).
		Value(&selected)
	if !allowEmpty {
		field = field.Validate(func(v []string) error {
			if len(v) == 0 {
				return fmt.Errorf("select at least one option")
			}
			return nil
		})
	}

	form := huh.NewForm(huh.NewGroup(field)).WithShowHelp(true)
	if in != nil {
		form = form.WithInput(in)
	}
	if out != nil {
		form = form.WithOutput(out)
	}

	if err := runForm(form); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, ErrCancelled
		}
		return nil, err
	}
	return selected, nil
}

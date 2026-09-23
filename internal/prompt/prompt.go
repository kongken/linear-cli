package prompt

import (
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/mattn/go-isatty"
	"github.com/kongken/linear-cli/internal/errors"
)

// IsInteractive reports whether stdin is a terminal.
func IsInteractive() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())
}

// Confirm asks a yes/no question when stdin is a TTY.
// When skip is true, returns true without prompting.
// When non-interactive and skip is false, returns a ValidationError suggesting flagName (default --force).
func Confirm(message string, skip bool) (bool, error) {
	return ConfirmWithFlag(message, skip, "--force")
}

// ConfirmWithFlag is like Confirm but names the skip flag in the error suggestion.
func ConfirmWithFlag(message string, skip bool, flagName string) (bool, error) {
	if skip {
		return true, nil
	}
	if !IsInteractive() {
		if flagName == "" {
			flagName = "--force"
		}
		return false, errors.NewValidationError(
			"Interactive confirmation required",
			errors.WithSuggestion(fmt.Sprintf("Use %s to skip confirmation.", flagName)),
		)
	}
	var confirmed bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(message).
				Value(&confirmed),
		),
	)
	if err := form.Run(); err != nil {
		return false, err
	}
	return confirmed, nil
}

// Text prompts for a string value when interactive.
func Text(message, value string) (string, error) {
	if value != "" {
		return value, nil
	}
	if !IsInteractive() {
		return "", errors.NewValidationError(
			message+" is required",
			errors.WithSuggestion("Pass the corresponding flag, or run in a terminal for interactive mode."),
		)
	}
	result := value
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(message).
				Value(&result),
		),
	)
	if err := form.Run(); err != nil {
		return "", err
	}
	return result, nil
}

// Option is a labeled selectable value for Select.
type Option struct {
	Label string
	Value string
}

// Select prompts the user to pick one option when interactive.
func Select(message string, options []Option) (string, error) {
	if len(options) == 0 {
		return "", errors.NewValidationError("No options available to select")
	}
	if !IsInteractive() {
		return "", errors.NewValidationError(
			"Interactive selection required",
			errors.WithSuggestion("Pass an explicit argument, or run in a terminal for interactive mode."),
		)
	}
	huhOpts := make([]huh.Option[string], 0, len(options))
	for _, o := range options {
		huhOpts = append(huhOpts, huh.NewOption(o.Label, o.Value))
	}
	var selected string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(message).
				Options(huhOpts...).
				Value(&selected),
		),
	)
	if err := form.Run(); err != nil {
		return "", err
	}
	return selected, nil
}

// MultiSelect prompts for zero or more options when interactive.
func MultiSelect(message string, options []Option) ([]string, error) {
	if len(options) == 0 {
		return nil, nil
	}
	if !IsInteractive() {
		return nil, errors.NewValidationError(
			"Interactive selection required",
			errors.WithSuggestion("Pass explicit flags, or run in a terminal for interactive mode."),
		)
	}
	huhOpts := make([]huh.Option[string], 0, len(options))
	for _, o := range options {
		huhOpts = append(huhOpts, huh.NewOption(o.Label, o.Value))
	}
	var selected []string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title(message).
				Options(huhOpts...).
				Value(&selected),
		),
	)
	if err := form.Run(); err != nil {
		return nil, err
	}
	return selected, nil
}

package editor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kongken/linear-cli/internal/errors"
)

// Get returns the preferred editor binary/command from git config or $EDITOR.
// Returns empty string when none is configured.
func Get() string {
	if out, err := exec.Command("git", "config", "--global", "core.editor").Output(); err == nil {
		if ed := strings.TrimSpace(string(out)); ed != "" {
			return ed
		}
	}
	if ed := strings.TrimSpace(os.Getenv("EDITOR")); ed != "" {
		return ed
	}
	return ""
}

// DisplayName returns the basename of the configured editor, or empty if none.
func DisplayName() string {
	ed := Get()
	if ed == "" {
		return ""
	}
	parts := strings.Fields(ed)
	return filepath.Base(parts[0])
}

// Open launches the editor on an empty temp markdown file and returns trimmed content.
// Empty content after edit returns "" with a nil error.
func Open() (string, error) {
	return OpenWithContent("")
}

// OpenWithContent writes initial content to a temp file, opens the editor, and returns trimmed result.
func OpenWithContent(initial string) (string, error) {
	ed := Get()
	if ed == "" {
		return "", errors.NewValidationError(
			"No editor found",
			errors.WithSuggestion("Set EDITOR environment variable or configure git editor with: git config --global core.editor <editor>"),
		)
	}

	f, err := os.CreateTemp("", "linear-cli-*.md")
	if err != nil {
		return "", errors.NewCliError("Failed to create temp file for editor", errors.WithCause(err))
	}
	path := f.Name()
	defer os.Remove(path)

	if initial != "" {
		if _, err := f.WriteString(initial); err != nil {
			_ = f.Close()
			return "", errors.NewCliError("Failed to write temp file for editor", errors.WithCause(err))
		}
	}
	if err := f.Close(); err != nil {
		return "", errors.NewCliError("Failed to close temp file for editor", errors.WithCause(err))
	}

	parts := strings.Fields(ed)
	args := append(append([]string{}, parts[1:]...), path)
	cmd := exec.Command(parts[0], args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", errors.NewCliError(fmt.Sprintf("Failed to open editor: %v", err), errors.WithCause(err))
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return "", errors.NewCliError("Failed to read editor output", errors.WithCause(err))
	}
	return strings.TrimSpace(string(b)), nil
}

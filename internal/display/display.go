package display

import (
	"fmt"
	"os"

	"github.com/charmbracelet/glamour"
	"github.com/mattn/go-isatty"
)

// PriorityDisplay returns the priority glyph for a Linear priority (0–4).
func PriorityDisplay(priority float64) string {
	switch int(priority) {
	case 0:
		return "---"
	case 1:
		return "⚠⚠⚠"
	case 2:
		return "▄▆█"
	case 3:
		return "▄▆ "
	case 4:
		return "▄  "
	default:
		return fmt.Sprintf("%v", priority)
	}
}

// IsTerminal reports whether w (usually os.Stdout) is an interactive terminal.
func IsTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}

// Markdown renders markdown for the terminal when stdout is a TTY; otherwise returns md unchanged.
func Markdown(md string) (string, error) {
	if !IsTerminal(os.Stdout) || os.Getenv("NO_COLOR") != "" {
		return md, nil
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(0),
	)
	if err != nil {
		return md, nil
	}
	out, err := r.Render(md)
	if err != nil {
		return md, nil
	}
	return out, nil
}

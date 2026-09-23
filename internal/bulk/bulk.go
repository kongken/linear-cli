package bulk

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kongken/linear-cli/internal/errors"
)

// Sources describes how IDs are supplied for a bulk operation.
type Sources struct {
	Bulk       []string
	File       string
	Stdin      bool
	Positional string
}

// IsMode reports whether any bulk source is set.
func IsMode(s Sources) bool {
	return len(s.Bulk) > 0 || s.File != "" || s.Stdin
}

// ParseIDs splits newline/comma/whitespace separated identifiers.
func ParseIDs(input string) []string {
	parts := strings.FieldsFunc(input, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ' ' || r == '\t'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Collect gathers unique IDs from bulk sources.
func Collect(s Sources) ([]string, error) {
	if !IsMode(s) {
		return nil, errors.NewValidationError("No bulk IDs provided")
	}
	if s.Positional != "" {
		return nil, errors.NewValidationError(
			"Cannot combine a positional ID with --bulk",
			errors.WithSuggestion("Pass every identifier through --bulk (or --bulk-file / --bulk-stdin), or drop the positional one."),
		)
	}
	var ids []string
	if len(s.Bulk) > 0 {
		ids = append(ids, s.Bulk...)
	}
	if s.File != "" {
		b, err := os.ReadFile(s.File)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, errors.NewNotFoundError("File", s.File)
			}
			return nil, errors.NewCliError("Failed to read bulk file", errors.WithCause(err))
		}
		ids = append(ids, ParseIDs(string(b))...)
	}
	if s.Stdin {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, errors.NewCliError("Failed to read stdin", errors.WithCause(err))
		}
		ids = append(ids, ParseIDs(string(b))...)
	}
	if len(ids) == 0 {
		return nil, errors.NewValidationError("No bulk IDs provided")
	}
	seen := map[string]struct{}{}
	uniq := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	return uniq, nil
}

// Result is one item from a bulk run.
type Result struct {
	ID      string
	Success bool
	Error   string
}

// PrintSummary writes a bulk summary to w.
func PrintSummary(w io.Writer, results []Result) {
	ok, fail := 0, 0
	for _, r := range results {
		if r.Success {
			ok++
		} else {
			fail++
		}
	}
	fmt.Fprintf(w, "Bulk complete: %d succeeded, %d failed (of %d)\n", ok, fail, len(results))
	for _, r := range results {
		if !r.Success {
			fmt.Fprintf(w, "  ✗ %s: %s\n", r.ID, r.Error)
		}
	}
}

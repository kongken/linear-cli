package display_test

import (
	"testing"

	"github.com/kongken/linear-cli/internal/display"
)

func TestPriorityDisplay(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "---"},
		{1, "⚠⚠⚠"},
		{2, "▄▆█"},
		{3, "▄▆ "},
		{4, "▄  "},
		{9, "9"},
	}
	for _, c := range cases {
		if got := display.PriorityDisplay(c.in); got != c.want {
			t.Fatalf("priority %v: got %q want %q", c.in, got, c.want)
		}
	}
}

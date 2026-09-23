package vcs_test

import (
	"testing"

	"github.com/kongken/linear-cli/internal/vcs"
)

func TestParseJjTrailersOutput(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"[ABC-123](https://linear.app/workspace/issue/ABC-123/some-title)\n\n", "ABC-123"},
		{"[abc-456](https://linear.app/workspace/issue/abc-456/some-title)\n", "ABC-456"},
		{"[PLA4-16916](https://linear.app/workspace/issue/PLA4-16916/x)\n", "PLA4-16916"},
		{"Fixes ABC-123\nhttps://linear.app/x\n\nother\n", "ABC-123"},
		{"ABC-123\n", "ABC-123"},
		{"[ABC-0123](https://linear.app/workspace/issue/ABC-0123/x)\n", ""},
		{"", ""},
		{"Closes ABC-456\n\n", "ABC-456"},
	}
	for _, c := range cases {
		if got := vcs.ParseJjTrailersOutput(c.in); got != c.want {
			t.Fatalf("%q: got %q want %q", c.in, got, c.want)
		}
	}
}

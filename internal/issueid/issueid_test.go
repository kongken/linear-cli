package issueid_test

import (
	"testing"

	"github.com/kongken/linear-cli/internal/issueid"
)

func TestParseIssueIdentifier(t *testing.T) {
	cases := []struct {
		in, id, team, num string
		ok                bool
	}{
		{"ABC-123", "ABC-123", "ABC", "123", true},
		{"PLA4-16916", "PLA4-16916", "PLA4", "16916", true},
		{"abc-123", "ABC-123", "ABC", "123", true},
		{"ABC-0123", "", "", "", false},
		{"123", "", "", "", false},
		{"", "", "", "", false},
		{"Fixes ABC-123", "", "", "", false},
	}
	for _, c := range cases {
		p, ok := issueid.ParseIssueIdentifier(c.in)
		if ok != c.ok {
			t.Fatalf("%q: ok=%v want %v", c.in, ok, c.ok)
		}
		if !c.ok {
			continue
		}
		if p.Identifier != c.id || p.TeamKey != c.team || p.IssueNumber != c.num {
			t.Fatalf("%q: %+v", c.in, p)
		}
	}
}

func TestFindIssueIdentifierInText(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"[ABC-123](https://linear.app/workspace/issue/ABC-123/some-title)", "ABC-123", true},
		{"[PLA4-16916](https://linear.app/workspace/issue/PLA4-16916/some-title)", "PLA4-16916", true},
		{"Fixes ABC-123", "ABC-123", true},
		{"feature/ABC-123-my-feature", "ABC-123", true},
		{"no issue here", "", false},
		{"ABC-0123", "", false},
	}
	for _, c := range cases {
		p, ok := issueid.FindIssueIdentifierInText(c.in)
		if ok != c.ok {
			t.Fatalf("%q: ok=%v want %v", c.in, ok, c.ok)
		}
		if c.ok && p.Identifier != c.want {
			t.Fatalf("%q: got %q want %q", c.in, p.Identifier, c.want)
		}
	}
}

func TestNormalizeIssueIdentifier(t *testing.T) {
	got, ok := issueid.NormalizeIssueIdentifier("eng-9")
	if !ok || got != "ENG-9" {
		t.Fatalf("%q %v", got, ok)
	}
}

func TestGetTeamKeyFromIssueIdentifier(t *testing.T) {
	if got := issueid.GetTeamKeyFromIssueIdentifier("ENG-42"); got != "ENG" {
		t.Fatalf("%q", got)
	}
}

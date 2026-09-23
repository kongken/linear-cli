package vcs

import (
	"os/exec"
	"strings"

	"github.com/kongken/linear-cli/internal/config"
	"github.com/kongken/linear-cli/internal/git"
	"github.com/kongken/linear-cli/internal/issueid"
)

// Type is the configured VCS backend.
type Type string

const (
	Git Type = "git"
	JJ  Type = "jj"
)

// Current returns the configured VCS (default git).
func Current() Type {
	if v, ok := config.GetOption("vcs"); ok && v != "" {
		return Type(v)
	}
	return Git
}

// CurrentIssueIdentifier returns the issue id from VCS state, or empty if none.
func CurrentIssueIdentifier() (string, error) {
	switch Current() {
	case Git:
		branch, err := git.CurrentBranch()
		if err != nil {
			return "", err
		}
		if branch == "" {
			return "", nil
		}
		if p, ok := issueid.FindIssueIdentifierInText(branch); ok {
			return p.Identifier, nil
		}
		return "", nil
	case JJ:
		return jjLinearIssue()
	default:
		return "", nil
	}
}

func jjLinearIssue() (string, error) {
	cmd := exec.Command(
		"jj", "log", "-r", "::@",
		"-T", `trailers.map(|t| if(t.key() == "Linear-issue", t.value(), ""))`,
		"--no-graph",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", nil
	}
	return parseJjTrailersOutput(string(out)), nil
}

// ParseJjTrailersOutput extracts the issue id from jj trailer log output.
func ParseJjTrailersOutput(output string) string {
	return parseJjTrailersOutput(output)
}

func parseJjTrailersOutput(output string) string {
	lines := strings.Split(output, "\n")
	var lastValid string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			if p, ok := issueid.FindIssueIdentifierInText(trimmed); ok {
				lastValid = p.Identifier
			}
		} else if lastValid != "" {
			return lastValid
		}
	}
	return lastValid
}

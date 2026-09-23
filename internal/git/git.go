package git

import (
	"os/exec"
	"strings"

	"github.com/kongken/linear-cli/internal/errors"
)

// CurrentBranch returns the short symbolic ref for HEAD, or "" if detached.
func CurrentBranch() (string, error) {
	cmd := exec.Command("git", "symbolic-ref", "--short", "HEAD")
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if strings.Contains(msg, "not a symbolic ref") {
			return "", nil
		}
		if msg == "" {
			msg = err.Error()
		}
		return "", errors.NewCliError("Failed to get current branch: "+msg, errors.WithCause(err))
	}
	return strings.TrimSpace(string(out)), nil
}

package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/kongken/linear-cli/internal/cli"
	"github.com/kongken/linear-cli/internal/config"
)

func TestRootHelpListsLinear(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := cli.NewRoot()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "linear") {
		t.Fatalf("help missing name: %s", out)
	}
}

func TestRootRegistersCommandGroups(t *testing.T) {
	cmd := cli.NewRoot()
	want := []string{
		"auth", "issue", "team", "user", "project", "project-update",
		"cycle", "milestone", "initiative", "initiative-update",
		"label", "template", "document", "config", "schema", "api", "markdown",
	}
	for _, name := range want {
		found := false
		for _, c := range cmd.Commands() {
			if c.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing command group %q", name)
		}
	}
	issue, _, err := cmd.Find([]string{"i"})
	if err != nil || issue.Name() != "issue" {
		t.Fatalf("alias i -> issue failed: %v name=%v", err, issue)
	}
}

func TestWorkspaceFlagSetsConfig(t *testing.T) {
	config.SetCLIWorkspace("")
	cmd := cli.NewRoot()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"--workspace", "acme"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := config.CLIWorkspace(); got != "acme" {
		t.Fatalf("CLIWorkspace = %q, want acme", got)
	}
}

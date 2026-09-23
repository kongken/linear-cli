package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kongken/linear-cli/internal/config"
)

func TestCLIWorkspaceRoundTrip(t *testing.T) {
	config.SetCLIWorkspace("")
	if config.CLIWorkspace() != "" {
		t.Fatal("expected empty")
	}
	config.SetCLIWorkspace("acme")
	if config.CLIWorkspace() != "acme" {
		t.Fatalf("got %q", config.CLIWorkspace())
	}
}

func TestGetOptionFromLinearToml(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".linear.toml")
	if err := os.WriteFile(path, []byte("team_id = \"ENG\"\nworkspace = \"acme\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.LoadProjectConfig(path); err != nil {
		t.Fatal(err)
	}
	team, ok := config.GetOption("team_id")
	if !ok || team != "ENG" {
		t.Fatalf("team_id = %q ok=%v", team, ok)
	}
	ws, ok := config.GetOption("workspace")
	if !ok || ws != "acme" {
		t.Fatalf("workspace = %q ok=%v", ws, ok)
	}
	if _, ok := config.GetOption("missing"); ok {
		t.Fatal("expected missing key")
	}
}

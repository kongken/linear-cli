package graphql_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kongken/linear-cli/internal/config"
	"github.com/kongken/linear-cli/internal/credentials"
	"github.com/kongken/linear-cli/internal/graphql"
)

func TestResolvedAPIKeyEnvWins(t *testing.T) {
	credentials.ResetForTest()
	config.SetCLIWorkspace("")
	t.Setenv("LINEAR_API_KEY", "lin_api_env")
	key, err := graphql.ResolvedAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if key != "lin_api_env" {
		t.Fatalf("got %q", key)
	}
}

func TestResolvedAPIKeyEnvWorkspaceConflict(t *testing.T) {
	credentials.ResetForTest()
	config.SetCLIWorkspace("acme")
	t.Setenv("LINEAR_API_KEY", "lin_api_env")
	_, err := graphql.ResolvedAPIKey()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Cannot use --workspace") {
		t.Fatalf("got %v", err)
	}
}

func TestResolvedAPIKeyFromCredentials(t *testing.T) {
	credentials.ResetForTest()
	config.SetCLIWorkspace("acme")
	os.Unsetenv("LINEAR_API_KEY")
	t.Setenv("LINEAR_API_KEY", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.toml")
	if err := os.WriteFile(path, []byte("acme = \"lin_api_acme\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := credentials.LoadFile(path); err != nil {
		t.Fatal(err)
	}
	key, err := graphql.ResolvedAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if key != "lin_api_acme" {
		t.Fatalf("got %q", key)
	}
}

func TestResolvedAPIKeyFromProjectWorkspace(t *testing.T) {
	credentials.ResetForTest()
	config.SetCLIWorkspace("")
	t.Setenv("LINEAR_API_KEY", "")
	os.Unsetenv("LINEAR_API_KEY")

	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials.toml")
	if err := os.WriteFile(credPath, []byte("default = \"acme\"\nacme = \"lin_api_from_project\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	credentials.SetPathOverride(credPath)
	if _, err := credentials.Load(); err != nil {
		t.Fatal(err)
	}

	tomlPath := filepath.Join(dir, ".linear.toml")
	if err := os.WriteFile(tomlPath, []byte("workspace = \"acme\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.LoadProjectConfig(tomlPath); err != nil {
		t.Fatal(err)
	}

	key, err := graphql.ResolvedAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if key != "lin_api_from_project" {
		t.Fatalf("got %q", key)
	}
}

func TestResolvedAPIKeyUnknownWorkspace(t *testing.T) {
	credentials.ResetForTest()
	config.SetCLIWorkspace("missing")
	t.Setenv("LINEAR_API_KEY", "")
	os.Unsetenv("LINEAR_API_KEY")
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.toml")
	if err := os.WriteFile(path, []byte("acme = \"lin_api_acme\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := credentials.LoadFile(path); err != nil {
		t.Fatal(err)
	}
	_, err := graphql.ResolvedAPIKey()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not found in credentials") {
		t.Fatalf("got %v", err)
	}
}

func TestResolvedAPIKeyMissing(t *testing.T) {
	credentials.ResetForTest()
	config.SetCLIWorkspace("")
	t.Setenv("LINEAR_API_KEY", "")
	os.Unsetenv("LINEAR_API_KEY")
	_ = config.LoadProjectConfig() // clear
	_, err := graphql.ResolvedAPIKey()
	if err == nil {
		t.Fatal("expected auth error")
	}
	if !strings.Contains(err.Error(), "No API key configured") {
		t.Fatalf("got %v", err)
	}
}

func TestEndpointOverride(t *testing.T) {
	t.Setenv("LINEAR_GRAPHQL_ENDPOINT", "http://127.0.0.1:9/graphql")
	if graphql.Endpoint() != "http://127.0.0.1:9/graphql" {
		t.Fatalf("got %q", graphql.Endpoint())
	}
}

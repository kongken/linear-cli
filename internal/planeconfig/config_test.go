package planeconfig_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kongken/workctl/internal/planeconfig"
)

func TestProfileCannotSendCredentialToOverriddenInstance(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config.toml")
	err := os.WriteFile(config, []byte("schema_version = 1\n[plane]\ndefault_profile = 'cloud'\n[plane.profiles.cloud]\nbase_url = 'https://api.plane.so'\nworkspace = 'acme'\napi_version = 'v2'\n"), 0600)
	if err != nil {
		t.Fatal(err)
	}
	resolver := planeconfig.Resolver{UserFile: config, ProjectDir: dir, Getenv: func(key string) string {
		if key == "PLANE_BASE_URL" {
			return "https://another.example"
		}
		return ""
	}}
	_, err = resolver.Resolve(planeconfig.Options{})
	if err == nil || !strings.Contains(err.Error(), "profile") {
		t.Fatalf("expected profile identity conflict, got %v", err)
	}
}

func TestProfileRoundTripAndProjectPrecedence(t *testing.T) {
	dir := t.TempDir()
	r := planeconfig.Resolver{UserFile: filepath.Join(dir, "config.toml"), ProjectDir: dir, Getenv: func(key string) string {
		if key == "PLANE_PROJECT_ID" {
			return "environment-project"
		}
		return ""
	}}
	p := planeconfig.Profile{BaseURL: "https://plane.example/prefix/", Workspace: "eng", APIVersion: "v1", ProjectID: "profile-project"}
	if err := r.SaveProfile("onprem", p); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "workctl.toml"), []byte("[plane]\nproject_id='repo-project'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := r.Resolve(planeconfig.Options{})
	if err != nil || c.BaseURL != "https://plane.example/prefix" || c.ProjectID != "environment-project" {
		t.Fatalf("context=%+v err=%v", c, err)
	}
	c, err = r.Resolve(planeconfig.Options{ProjectID: "flag-project"})
	if err != nil || c.ProjectID != "flag-project" {
		t.Fatalf("flag precedence: %+v %v", c, err)
	}
	p.BaseURL = "https://other.example"
	if err := r.SaveProfile("onprem", p); err == nil {
		t.Fatal("profile identity could be overwritten")
	}
}

func TestInvalidConfigFailsInsteadOfUsingDefaults(t *testing.T) {
	for _, text := range []string{"schema_version=2\n", "schema_version=1\n[plane]\napi_key='secret'\n"} {
		dir := t.TempDir()
		file := filepath.Join(dir, "config.toml")
		if err := os.WriteFile(file, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		r := planeconfig.Resolver{UserFile: file, ProjectDir: dir, Getenv: func(string) string { return "" }}
		if _, err := r.Resolve(planeconfig.Options{Workspace: "eng"}); err == nil {
			t.Fatalf("accepted config %q", text)
		}
	}
}

package credentials_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kongken/linear-cli/internal/credentials"
)

func TestLoadInlineCredentials(t *testing.T) {
	credentials.ResetForTest()
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.toml")
	content := `
default = "acme"
acme = "lin_api_acme"
other = "lin_api_other"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	creds, err := credentials.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if creds.Default != "acme" {
		t.Fatalf("default = %q", creds.Default)
	}
	if len(creds.Workspaces) != 2 {
		t.Fatalf("workspaces = %#v", creds.Workspaces)
	}
	key, ok := credentials.GetAPIKey("acme")
	if !ok || key != "lin_api_acme" {
		t.Fatalf("acme key = %q ok=%v", key, ok)
	}
}

func TestCredentialsPathUsesXDG(t *testing.T) {
	credentials.ResetForTest()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path, err := credentials.CredentialsPath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "linear", "credentials.toml")
	if path != want {
		t.Fatalf("got %q want %q", path, want)
	}
}

func TestAddCredentialInline(t *testing.T) {
	credentials.ResetForTest()
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.toml")
	credentials.SetPathOverride(path)

	if err := credentials.AddCredential("acme", "lin_api_1", true); err != nil {
		t.Fatal(err)
	}
	if !credentials.HasWorkspace("acme") {
		t.Fatal("expected workspace")
	}
	if credentials.DefaultWorkspace() != "acme" {
		t.Fatalf("default=%q", credentials.DefaultWorkspace())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("expected file written")
	}

	credentials.ResetForTest()
	credentials.SetPathOverride(path)
	if _, err := credentials.Load(); err != nil {
		t.Fatal(err)
	}
	key, ok := credentials.GetAPIKey("acme")
	if !ok || key != "lin_api_1" {
		t.Fatalf("reload key=%q ok=%v", key, ok)
	}
}

func TestLoadKeyringFormatCredentialsFile(t *testing.T) {
	// Keyring format: workspaces = ["acme"] with keys in OS keyring.
	credentials.ResetForTest()
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.toml")
	content := `
default = "acme"
workspaces = ["acme", "other"]
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	creds, err := credentials.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if credentials.UsingInlineFormat() {
		t.Fatal("expected keyring format, got inline")
	}
	if creds.Default != "acme" {
		t.Fatalf("default=%q", creds.Default)
	}
	if !credentials.HasWorkspace("acme") || !credentials.HasWorkspace("other") {
		t.Fatalf("workspaces=%v", credentials.Workspaces())
	}
}

func TestInlineCredentialsRoundTrip(t *testing.T) {
	// Shape written for plaintext credentials.toml
	credentials.ResetForTest()
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.toml")
	inline := "default = \"acme\"\nacme = \"lin_api_test\"\nother = \"lin_api_other\"\n"
	if err := os.WriteFile(path, []byte(inline), 0o600); err != nil {
		t.Fatal(err)
	}
	credentials.SetPathOverride(path)
	if _, err := credentials.Load(); err != nil {
		t.Fatal(err)
	}
	key, ok := credentials.GetAPIKey("acme")
	if !ok || key != "lin_api_test" {
		t.Fatalf("acme=%q ok=%v", key, ok)
	}
	key, ok = credentials.GetAPIKey("")
	if !ok || key != "lin_api_test" {
		t.Fatalf("default key=%q ok=%v", key, ok)
	}
	if !credentials.UsingInlineFormat() {
		t.Fatal("expected inline format")
	}
}


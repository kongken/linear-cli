package planeauth_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/kongken/workctl/internal/keyring"
	"github.com/kongken/workctl/internal/planeauth"
	"github.com/kongken/workctl/internal/planeconfig"
)

func TestCredentialsAreIsolatedByInstanceAndLogout(t *testing.T) {
	r := planeconfig.Resolver{UserFile: filepath.Join(t.TempDir(), "config.toml")}
	m := planeauth.Manager{Resolver: r, Store: keyring.NewMemoryStore()}
	a := planeconfig.Context{ProfileName: "cloud", Profile: planeconfig.Profile{BaseURL: "https://api.plane.so", Workspace: "acme", APIVersion: "v2"}}
	b := planeconfig.Context{ProfileName: "cloud", Profile: planeconfig.Profile{BaseURL: "https://other.example", Workspace: "acme", APIVersion: "v2"}}
	if err := m.Save(a, "token-a", false); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Token(b); err == nil {
		t.Fatal("another instance could read credential")
	}
	if got, err := m.Token(a); err != nil || got != "token-a" {
		t.Fatalf("token=%q err=%v", got, err)
	}
	if err := m.Logout(a); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Token(a); err == nil {
		t.Fatal("credential survives logout")
	}
}

type unavailableKeyring struct{}

func (unavailableKeyring) Get(string, string) (string, error) {
	return "", fmt.Errorf("keyring unavailable")
}
func (unavailableKeyring) Set(string, string, string) error { return fmt.Errorf("keyring unavailable") }
func (unavailableKeyring) Delete(string, string) error      { return fmt.Errorf("keyring unavailable") }
func TestPlaintextLogoutWorksWithoutSystemKeyring(t *testing.T) {
	r := planeconfig.Resolver{UserFile: filepath.Join(t.TempDir(), "config.toml")}
	m := planeauth.Manager{Resolver: r, Store: unavailableKeyring{}}
	c := planeconfig.Context{ProfileName: "headless", Profile: planeconfig.Profile{BaseURL: "https://api.plane.so", Workspace: "eng", APIVersion: "v2"}}
	if err := m.Save(c, "private-plaintext-token", true); err != nil {
		t.Fatal(err)
	}
	if err := m.Logout(c); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Token(c); err == nil {
		t.Fatal("logged-out plaintext credential returned")
	}
}

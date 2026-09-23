package cmdauth_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kongken/linear-cli/internal/cmdauth"
	"github.com/kongken/linear-cli/internal/credentials"
)

func TestAuthLoginAndWhoami(t *testing.T) {
	credentials.ResetForTest()
	dir := t.TempDir()
	credentials.SetPathOverride(filepath.Join(dir, "credentials.toml"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"viewer": map[string]any{
					"id":          "u1",
					"name":        "Ada",
					"displayName": "Ada",
					"email":       "ada@example.com",
					"admin":       true,
					"guest":       false,
					"organization": map[string]any{
						"name":   "Acme",
						"urlKey": "acme",
					},
				},
			},
		})
	}))
	defer srv.Close()
	t.Setenv("LINEAR_GRAPHQL_ENDPOINT", srv.URL)
	t.Setenv("LINEAR_API_KEY", "")

	out := new(bytes.Buffer)
	cmd := cmdauth.New()
	cmd.SetOut(out)
	cmd.SetArgs([]string{"login", "--key", "lin_api_test", "--plaintext"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Logged in to workspace: Acme (acme)") {
		t.Fatalf("login output: %s", out.String())
	}
	if !credentials.HasWorkspace("acme") {
		t.Fatal("workspace not saved")
	}

	t.Setenv("LINEAR_API_KEY", "lin_api_test")
	out.Reset()
	cmd = cmdauth.New()
	cmd.SetOut(out)
	cmd.SetArgs([]string{"whoami"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Workspace: Acme") {
		t.Fatalf("whoami: %s", out.String())
	}
}

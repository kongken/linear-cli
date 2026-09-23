package cmdissue_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kongken/linear-cli/internal/cli"
	"github.com/kongken/linear-cli/internal/config"
)

func TestIssueMineJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"issues": map[string]any{
					"nodes": []any{
						map[string]any{
							"id": "1", "identifier": "ENG-1", "title": "Login",
							"priority": 2,
							"state":    map[string]any{"name": "Todo", "type": "unstarted"},
						},
					},
					"pageInfo": map[string]any{"hasNextPage": false, "endCursor": nil},
				},
			},
		})
	}))
	defer srv.Close()
	t.Setenv("LINEAR_API_KEY", "k")
	t.Setenv("LINEAR_GRAPHQL_ENDPOINT", srv.URL)
	t.Setenv("NO_COLOR", "1")

	dir := t.TempDir()
	tomlPath := filepath.Join(dir, ".linear.toml")
	if err := os.WriteFile(tomlPath, []byte("team_id = \"ENG\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	if err := config.LoadProjectConfig(tomlPath); err != nil {
		t.Fatal(err)
	}

	root := cli.NewRoot()
	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(errBuf)
	root.SetArgs([]string{"issue", "mine", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v stderr=%s", err, errBuf)
	}
	out := buf.String()
	if !strings.Contains(out, `"nodes"`) || !strings.Contains(out, "ENG-1") || !strings.Contains(out, `"pageInfo"`) {
		t.Fatalf("unexpected json: %s", out)
	}
}

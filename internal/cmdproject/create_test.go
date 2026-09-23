package cmdproject_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kongken/linear-cli/internal/cli"
)

func TestProjectCreateHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query string `json:"query"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch {
		case strings.Contains(body.Query, "ResolveTeam"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"teams": map[string]any{
						"nodes": []any{map[string]any{"id": "team1", "key": "ENG"}},
					},
				},
			})
		case strings.Contains(body.Query, "projectCreate"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"projectCreate": map[string]any{
						"success": true,
						"project": map[string]any{
							"id": "p1", "name": "Alpha", "slugId": "alpha",
							"url": "https://linear.app/x/project/alpha",
						},
					},
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{}})
		}
	}))
	defer srv.Close()
	t.Setenv("LINEAR_API_KEY", "k")
	t.Setenv("LINEAR_GRAPHQL_ENDPOINT", srv.URL)
	t.Setenv("NO_COLOR", "1")

	root := cli.NewRoot()
	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(errBuf)
	root.SetArgs([]string{
		"project", "create",
		"--name", "Alpha",
		"--team", "ENG",
		"--priority", "high",
		"--no-interactive",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v stderr=%s", err, errBuf)
	}
	out := buf.String()
	if !strings.Contains(out, "Alpha") || !strings.Contains(out, "alpha") {
		t.Fatalf("unexpected output: %s", out)
	}
}

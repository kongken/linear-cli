package cmdprojectupdate_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kongken/linear-cli/internal/cli"
)

func TestProjectUpdateCreate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query string `json:"query"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch {
		case strings.Contains(body.Query, "projectUpdateCreate") || strings.Contains(body.Query, "ProjectUpdateCreate"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"projectUpdateCreate": map[string]any{
						"success": true,
						"projectUpdate": map[string]any{
							"url": "https://linear.app/x/project/p/update/1",
						},
					},
				},
			})
		case strings.Contains(body.Query, "project(") || strings.Contains(body.Query, "projects("):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"project": map[string]any{"id": "proj-1", "name": "P", "slugId": "p"},
					"projects": map[string]any{
						"nodes": []any{map[string]any{"id": "proj-1", "name": "P", "slugId": "p"}},
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
	root.SetArgs([]string{"project-update", "create", "proj-1", "--body", "Ship it"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v stderr=%s", err, errBuf)
	}
	if !strings.Contains(buf.String(), "https://linear.app") {
		t.Fatalf("unexpected: %s stderr=%s", buf.String(), errBuf.String())
	}
}

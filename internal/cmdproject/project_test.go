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

func TestProjectListJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"projects": map[string]any{
					"nodes": []any{
						map[string]any{
							"id": "p1", "name": "Alpha", "description": nil, "slugId": "alpha",
							"url": "https://linear.app/x", "status": map[string]any{"id": "s", "name": "Planned", "color": "#000", "type": "planned"},
							"lead": nil, "teams": map[string]any{"nodes": []any{}},
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

	root := cli.NewRoot()
	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(errBuf)
	root.SetArgs([]string{"project", "list", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v stderr=%s", err, errBuf)
	}
	out := buf.String()
	if !strings.Contains(out, `"name"`) || !strings.Contains(out, "Alpha") {
		t.Fatalf("unexpected json: %s", out)
	}
	if !strings.Contains(out, `"nodes"`) || !strings.Contains(out, `"pageInfo"`) {
		t.Fatalf("expected connection shape, got: %s", out)
	}
}

package cmdlabel_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kongken/linear-cli/internal/cli"
)

func TestLabelListJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"issueLabels": map[string]any{
					"nodes": []any{
						map[string]any{
							"id": "l1", "name": "bug", "description": nil, "color": "#f00",
							"team": map[string]any{"key": "ENG", "name": "Engineering"},
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
	root.SetArgs([]string{"label", "list", "--team", "ENG", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v stderr=%s", err, errBuf)
	}
	out := buf.String()
	if !strings.Contains(out, `"nodes"`) || !strings.Contains(out, "bug") || !strings.Contains(out, `"pageInfo"`) {
		t.Fatalf("unexpected json: %s", out)
	}
}

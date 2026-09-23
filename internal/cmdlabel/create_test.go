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

func TestLabelCreate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query string `json:"query"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch {
		case strings.Contains(body.Query, "ResolveTeam") || strings.Contains(body.Query, "teams("):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"teams": map[string]any{
						"nodes": []any{map[string]any{"id": "team1", "key": "ENG", "name": "Engineering"}},
					},
				},
			})
		case strings.Contains(body.Query, "issueLabelCreate") || strings.Contains(body.Query, "LabelCreate"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"issueLabelCreate": map[string]any{
						"success": true,
						"issueLabel": map[string]any{
							"id": "l1", "name": "bug", "color": "#f00",
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
	root.SetArgs([]string{"label", "create", "--name", "bug", "--team", "ENG", "--color", "#ff0000"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v stderr=%s", err, errBuf)
	}
	if !strings.Contains(buf.String(), "bug") {
		t.Fatalf("unexpected: %s stderr=%s", buf.String(), errBuf.String())
	}
}

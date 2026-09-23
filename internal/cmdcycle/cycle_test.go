package cmdcycle_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kongken/linear-cli/internal/cli"
)

func TestCycleListJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query string `json:"query"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if strings.Contains(body.Query, "ResolveTeam") || strings.Contains(body.Query, "teams(") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"teams": map[string]any{
						"nodes": []any{map[string]any{"id": "team1", "key": "ENG", "name": "Engineering"}},
					},
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"team": map[string]any{
					"cycles": map[string]any{
						"nodes": []any{
							map[string]any{
								"id": "c1", "number": 12, "name": "Sprint 12",
								"startsAt": "2026-01-01T00:00:00Z", "endsAt": "2026-01-14T00:00:00Z",
								"completedAt": nil, "isActive": true, "isNext": false, "isPrevious": false,
							},
						},
						"pageInfo": map[string]any{"hasNextPage": false, "endCursor": nil},
					},
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
	root.SetArgs([]string{"cycle", "list", "--team", "ENG", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v stderr=%s", err, errBuf)
	}
	out := buf.String()
	if !strings.Contains(out, "Sprint 12") && !strings.Contains(out, `"number"`) {
		t.Fatalf("unexpected json: %s", out)
	}
}

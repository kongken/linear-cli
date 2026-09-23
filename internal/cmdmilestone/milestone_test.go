package cmdmilestone_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kongken/linear-cli/internal/cli"
)

func TestMilestoneListJSON(t *testing.T) {
	projectID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"project": map[string]any{
					"id":   projectID,
					"name": "Alpha",
					"projectMilestones": map[string]any{
						"nodes": []any{
							map[string]any{
								"id": "m1", "name": "Phase 1", "targetDate": nil, "sortOrder": 1,
								"project": map[string]any{"id": projectID, "name": "Alpha"},
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
	root.SetArgs([]string{"milestone", "list", "--project", projectID, "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v stderr=%s", err, errBuf)
	}
	out := buf.String()
	if !strings.Contains(out, "Phase 1") {
		t.Fatalf("unexpected json: %s stderr=%s", out, errBuf)
	}
}

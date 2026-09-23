package cmdteam_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kongken/linear-cli/internal/cli"
)

func TestTeamListJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"teams": map[string]any{
					"nodes": []any{
						map[string]any{"id": "t1", "key": "ENG", "name": "Engineering"},
						map[string]any{"id": "t2", "key": "DES", "name": "Design"},
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
	root.SetArgs([]string{"team", "list", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v stderr=%s", err, errBuf)
	}
	out := buf.String()
	if !strings.Contains(out, `"nodes"`) || !strings.Contains(out, "ENG") || !strings.Contains(out, `"pageInfo"`) {
		t.Fatalf("unexpected json: %s", out)
	}
}

func TestTeamCreate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query string `json:"query"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if strings.Contains(body.Query, "teamCreate") || strings.Contains(body.Query, "CreateTeam") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"teamCreate": map[string]any{
						"success": true,
						"team":    map[string]any{"id": "t1", "key": "NEW", "name": "New Team"},
					},
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{}})
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
	root.SetArgs([]string{"team", "create", "--name", "New Team", "--key", "NEW"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v stderr=%s", err, errBuf)
	}
	if !strings.Contains(buf.String(), "NEW") || !strings.Contains(buf.String(), "New Team") {
		t.Fatalf("unexpected: %s", buf.String())
	}
}

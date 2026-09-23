package teams_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kongken/linear-cli/internal/teams"
)

func TestResolveTeam(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"teams": map[string]any{
					"nodes": []any{
						map[string]any{"id": "t1", "key": "ENG", "name": "Engineering"},
					},
				},
			},
		})
	}))
	defer srv.Close()
	t.Setenv("LINEAR_API_KEY", "k")
	t.Setenv("LINEAR_GRAPHQL_ENDPOINT", srv.URL)

	team, err := teams.Resolve(context.Background(), "eng")
	if err != nil {
		t.Fatal(err)
	}
	if team.Key != "ENG" || team.ID != "t1" {
		t.Fatalf("%+v", team)
	}
}

func TestListAllTeams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"teams": map[string]any{
					"nodes": []any{
						map[string]any{"id": "t1", "key": "ENG", "name": "Engineering"},
						map[string]any{"id": "t2", "key": "DES", "name": "Design"},
					},
					"pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""},
				},
			},
		})
	}))
	defer srv.Close()
	t.Setenv("LINEAR_API_KEY", "k")
	t.Setenv("LINEAR_GRAPHQL_ENDPOINT", srv.URL)

	all, err := teams.ListAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("%+v", all)
	}
}

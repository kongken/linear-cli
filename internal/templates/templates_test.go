package templates_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kongken/linear-cli/internal/templates"
)

func TestResolveTemplateByName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"templates": []any{
					map[string]any{
						"id": "tmpl-1", "name": "Bug", "type": "issue",
						"team": map[string]any{"id": "team-eng", "key": "ENG", "name": "Engineering"},
					},
					map[string]any{
						"id": "tmpl-2", "name": "Bug", "type": "project", "team": nil,
					},
				},
			},
		})
	}))
	defer srv.Close()
	t.Setenv("LINEAR_API_KEY", "k")
	t.Setenv("LINEAR_GRAPHQL_ENDPOINT", srv.URL)

	tmpl, err := templates.Resolve(context.Background(), "bug", &templates.Scope{
		Type:    "issue",
		TeamIDs: []string{"team-eng"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if tmpl.ID != "tmpl-1" {
		t.Fatalf("%+v", tmpl)
	}
}

func TestResolveTemplateByUUID(t *testing.T) {
	id := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"template": map[string]any{
					"id": id, "name": "Bug", "type": "issue", "team": nil,
				},
			},
		})
	}))
	defer srv.Close()
	t.Setenv("LINEAR_API_KEY", "k")
	t.Setenv("LINEAR_GRAPHQL_ENDPOINT", srv.URL)

	tmpl, err := templates.Resolve(context.Background(), id, &templates.Scope{
		Type:    "issue",
		TeamIDs: []string{"team-eng"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if tmpl.Name != "Bug" {
		t.Fatalf("%+v", tmpl)
	}
}

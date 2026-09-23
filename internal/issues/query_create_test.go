package issues_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kongken/linear-cli/internal/issues"
)

func TestFetchQueryIssuesAppliesFilters(t *testing.T) {
	var gotVars map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if strings.Contains(body.Query, "LookupUser") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"users": map[string]any{
						"nodes": []any{map[string]any{"id": "user-42"}},
					},
				},
			})
			return
		}
		if strings.Contains(body.Query, "ResolveProject") || strings.Contains(body.Query, "projects(") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"project":  map[string]any{"id": "proj-1"},
					"projects": map[string]any{"nodes": []any{map[string]any{"id": "proj-1", "name": "Ship", "slugId": "ship"}}},
				},
			})
			return
		}
		gotVars = body.Variables
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"issues": map[string]any{
					"nodes":    []any{},
					"pageInfo": map[string]any{"hasNextPage": false, "endCursor": nil},
				},
			},
		})
	}))
	defer srv.Close()
	t.Setenv("LINEAR_API_KEY", "k")
	t.Setenv("LINEAR_GRAPHQL_ENDPOINT", srv.URL)

	_, err := issues.FetchQueryIssues(context.Background(), issues.QueryOptions{
		TeamKeys:        []string{"ENG"},
		Assignee:        "alice",
		Project:         "Ship",
		Labels:          []string{"bug"},
		CreatedAfter:    "2024-01-01",
		IncludeArchived: true,
		Limit:           10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotVars["includeArchived"] != true {
		t.Fatalf("includeArchived=%v", gotVars["includeArchived"])
	}
	filter, _ := gotVars["filter"].(map[string]any)
	if filter == nil {
		t.Fatalf("missing filter: %#v", gotVars)
	}
	assignee, _ := filter["assignee"].(map[string]any)
	idFilter, _ := assignee["id"].(map[string]any)
	if idFilter["eq"] != "user-42" {
		t.Fatalf("assignee filter=%#v", filter["assignee"])
	}
	project, _ := filter["project"].(map[string]any)
	pid, _ := project["id"].(map[string]any)
	if pid["eq"] != "proj-1" {
		t.Fatalf("project filter=%#v", filter["project"])
	}
	labels, _ := filter["labels"].(map[string]any)
	some, _ := labels["some"].(map[string]any)
	name, _ := some["name"].(map[string]any)
	if name["eqIgnoreCase"] != "bug" {
		t.Fatalf("labels filter=%#v", filter["labels"])
	}
	created, _ := filter["createdAt"].(map[string]any)
	if created["gte"] == nil {
		t.Fatalf("createdAt filter=%#v", filter["createdAt"])
	}
}

func TestFetchQueryIssues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"issues": map[string]any{
					"nodes": []any{
						map[string]any{"identifier": "ENG-2", "title": "Query me", "state": map[string]any{"name": "Todo"}},
					},
					"pageInfo": map[string]any{"hasNextPage": false, "endCursor": nil},
				},
			},
		})
	}))
	defer srv.Close()
	t.Setenv("LINEAR_API_KEY", "k")
	t.Setenv("LINEAR_GRAPHQL_ENDPOINT", srv.URL)

	payload, err := issues.FetchQueryIssues(context.Background(), issues.QueryOptions{
		TeamKeys: []string{"ENG"},
		Limit:    10,
	})
	if err != nil {
		t.Fatal(err)
	}
	text, err := issues.FormatQueryText(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "ENG-2") {
		t.Fatalf("%s", text)
	}
}

func TestCreateIssue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query string `json:"query"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if strings.Contains(body.Query, "ResolveTeam") {
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
				"issueCreate": map[string]any{
					"success": true,
					"issue": map[string]any{
						"id": "i1", "identifier": "ENG-9", "url": "https://linear.app/x", "title": "New",
						"team": map[string]any{"key": "ENG"},
					},
				},
			},
		})
	}))
	defer srv.Close()
	t.Setenv("LINEAR_API_KEY", "k")
	t.Setenv("LINEAR_GRAPHQL_ENDPOINT", srv.URL)

	issue, err := issues.CreateIssue(context.Background(), issues.CreateInput{
		Title:   "New",
		TeamKey: "ENG",
	})
	if err != nil {
		t.Fatal(err)
	}
	if issue.Identifier != "ENG-9" {
		t.Fatalf("%+v", issue)
	}
}

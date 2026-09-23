package issues_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kongken/linear-cli/internal/graphql"
	"github.com/kongken/linear-cli/internal/issues"
)

func TestCreateIssueWithProjectAndLabels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query string `json:"query"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch {
		case strings.Contains(body.Query, "ResolveTeam"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"teams": map[string]any{
						"nodes": []any{map[string]any{"id": "team1", "key": "ENG", "name": "Engineering"}},
					},
				},
			})
		case strings.Contains(body.Query, "projects"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"projects": map[string]any{
						"nodes": []any{map[string]any{"id": "proj1"}},
					},
				},
			})
		case strings.Contains(body.Query, "issueLabels"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"issueLabels": map[string]any{
						"nodes": []any{map[string]any{"id": "lab1", "name": "bug", "team": map[string]any{"key": "ENG"}}},
					},
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"issueCreate": map[string]any{
						"success": true,
						"issue": map[string]any{
							"id": "i1", "identifier": "ENG-10", "url": "https://linear.app/x", "title": "Bug",
							"team": map[string]any{"key": "ENG"},
						},
					},
				},
			})
		}
	}))
	defer srv.Close()
	t.Setenv("LINEAR_API_KEY", "k")
	t.Setenv("LINEAR_GRAPHQL_ENDPOINT", srv.URL)

	issue, err := issues.CreateIssue(context.Background(), issues.CreateInput{
		Title:   "Bug",
		TeamKey: "ENG",
		Project: "My Project",
		Labels:  []string{"bug"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if issue.Identifier != "ENG-10" {
		t.Fatalf("got %s", issue.Identifier)
	}
}

func TestResolveWorkflowStateID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"team": map[string]any{
					"states": map[string]any{
						"nodes": []any{
							map[string]any{"id": "s1", "name": "In Progress", "type": "started"},
							map[string]any{"id": "s2", "name": "Todo", "type": "unstarted"},
						},
					},
				},
			},
		})
	}))
	defer srv.Close()
	t.Setenv("LINEAR_API_KEY", "k")
	t.Setenv("LINEAR_GRAPHQL_ENDPOINT", srv.URL)

	client, err := graphql.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	id, err := issues.ResolveWorkflowStateID(context.Background(), client, "team1", "started")
	if err != nil {
		t.Fatal(err)
	}
	if id != "s1" {
		t.Fatalf("got %s", id)
	}
}

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

func TestFetchMineIssues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"issues": map[string]any{
					"nodes": []any{
						map[string]any{
							"id":         "1",
							"identifier": "ENG-1",
							"title":      "Login",
							"priority":   2,
							"state":      map[string]any{"name": "Todo", "type": "unstarted"},
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

	payload, err := issues.FetchMineIssues(context.Background(), issues.MineOptions{
		TeamKey:   "ENG",
		StateType: "unstarted",
		Limit:     50,
	})
	if err != nil {
		t.Fatal(err)
	}
	text, err := issues.FormatMineText(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "ENG-1") || !strings.Contains(text, "Login") {
		t.Fatalf("%s", text)
	}
	summaries, err := issues.ParseIssueSummaries(payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 || summaries[0].Identifier != "ENG-1" || summaries[0].Priority != 2 {
		t.Fatalf("%+v", summaries)
	}
}

func TestFetchMineIssuesRejectsConflictingAssigneeFilters(t *testing.T) {
	_, err := issues.FetchMineIssues(context.Background(), issues.MineOptions{
		TeamKey:      "ENG",
		Unassigned:   true,
		AllAssignees: true,
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

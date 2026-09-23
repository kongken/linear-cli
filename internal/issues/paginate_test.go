package issues_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kongken/linear-cli/internal/issues"
)

func TestFetchMineIssuesPaginatesAndKeepsConnectionShape(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		hasAfter := strings.Contains(string(body), `"after"`)
		if n == 1 {
			if hasAfter {
				t.Errorf("first page should not send after")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"issues": map[string]any{
						"nodes": []any{
							map[string]any{"id": "1", "identifier": "ENG-1", "title": "A", "priority": 1},
						},
						"pageInfo": map[string]any{"hasNextPage": true, "endCursor": "cursor-1"},
					},
				},
			})
			return
		}
		if !hasAfter {
			t.Errorf("second page should send after")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"issues": map[string]any{
					"nodes": []any{
						map[string]any{"id": "2", "identifier": "ENG-2", "title": "B", "priority": 2},
					},
					"pageInfo": map[string]any{"hasNextPage": false, "endCursor": "cursor-2"},
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
	if calls.Load() < 2 {
		t.Fatalf("expected pagination, calls=%d", calls.Load())
	}
	var parsed struct {
		Nodes    []json.RawMessage `json:"nodes"`
		PageInfo json.RawMessage   `json:"pageInfo"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Nodes) != 2 {
		t.Fatalf("nodes=%d payload=%s", len(parsed.Nodes), payload)
	}
	if !strings.Contains(string(parsed.PageInfo), "hasNextPage") {
		t.Fatalf("pageInfo=%s", parsed.PageInfo)
	}
	joined := string(payload)
	if !strings.Contains(joined, "ENG-1") || !strings.Contains(joined, "ENG-2") {
		t.Fatalf("%s", joined)
	}
}

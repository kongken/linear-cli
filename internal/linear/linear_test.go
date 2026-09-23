package linear_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kongken/linear-cli/internal/linear"
)

func TestFetchIssueTitleURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"issue": map[string]any{
					"title":      "Fix login",
					"url":        "https://linear.app/acme/issue/ENG-1",
					"identifier": "ENG-1",
				},
			},
		})
	}))
	defer srv.Close()

	t.Setenv("LINEAR_API_KEY", "test-key")
	t.Setenv("LINEAR_GRAPHQL_ENDPOINT", srv.URL)

	issue, err := linear.FetchIssueTitleURL(context.Background(), "ENG-1")
	if err != nil {
		t.Fatal(err)
	}
	if issue.Title != "Fix login" || issue.URL == "" {
		t.Fatalf("%+v", issue)
	}
}

func TestFetchIssueDetailsJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"issue": map[string]any{
					"identifier":  "ENG-1",
					"title":       "Fix login",
					"description": "body",
					"url":         "https://linear.app/acme/issue/ENG-1",
					"state":       map[string]any{"name": "Todo", "color": "#000"},
					"labels":      map[string]any{"nodes": []any{}},
					"children":    map[string]any{"nodes": []any{}},
					"attachments": map[string]any{"nodes": []any{}},
					"documents":   map[string]any{"nodes": []any{}},
				},
			},
		})
	}))
	defer srv.Close()
	t.Setenv("LINEAR_API_KEY", "test-key")
	t.Setenv("LINEAR_GRAPHQL_ENDPOINT", srv.URL)

	raw, err := linear.FetchIssueDetailsJSON(context.Background(), "ENG-1")
	if err != nil {
		t.Fatal(err)
	}
	text, err := linear.FormatIssueDetailsText(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "# ENG-1 Fix login") {
		t.Fatalf("%s", text)
	}
}

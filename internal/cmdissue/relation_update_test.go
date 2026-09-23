package cmdissue_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kongken/linear-cli/internal/cli"
)

func TestIssueRelationList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query string `json:"query"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"issue": map[string]any{
					"identifier": "ENG-1",
					"title":      "Parent",
					"relations": map[string]any{
						"nodes": []any{
							map[string]any{
								"id":   "r1",
								"type": "blocks",
								"relatedIssue": map[string]any{
									"identifier": "ENG-2",
									"title":      "Child",
								},
							},
						},
					},
					"inverseRelations": map[string]any{"nodes": []any{}},
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
	root.SetOut(buf)
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"issue", "relation", "list", "ENG-1"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "ENG-1") || !strings.Contains(out, "blocks") || !strings.Contains(out, "ENG-2") {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestIssueUpdateAddLabel(t *testing.T) {
	var gotInput map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch {
		case strings.Contains(body.Query, "issueLabels") || strings.Contains(body.Query, "IssueLabel"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"issueLabels": map[string]any{
						"nodes": []any{map[string]any{"id": "lab-1", "name": "bug"}},
					},
				},
			})
		case strings.Contains(body.Query, "team { key }"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"issue": map[string]any{"team": map[string]any{"key": "ENG"}},
				},
			})
		case strings.Contains(body.Query, "issueUpdate") || strings.Contains(body.Query, "UpdateIssue"):
			if input, ok := body.Variables["input"].(map[string]any); ok {
				gotInput = input
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"issueUpdate": map[string]any{
						"success": true,
						"issue": map[string]any{
							"identifier": "ENG-1",
							"url":        "https://linear.app/x/ENG-1",
						},
					},
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"issue": map[string]any{"id": "i1", "identifier": "ENG-1"},
				},
			})
		}
	}))
	defer srv.Close()
	t.Setenv("LINEAR_API_KEY", "k")
	t.Setenv("LINEAR_GRAPHQL_ENDPOINT", srv.URL)

	root := cli.NewRoot()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"issue", "update", "ENG-1", "--add-label", "bug"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	ids, _ := gotInput["addedLabelIds"].([]any)
	if len(ids) == 0 {
		t.Fatalf("expected addedLabelIds, got %#v", gotInput)
	}
}

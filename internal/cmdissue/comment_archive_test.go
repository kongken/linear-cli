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

func TestIssueCommentAdd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query string `json:"query"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch {
		case strings.Contains(body.Query, "commentCreate"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"commentCreate": map[string]any{
						"success": true,
						"comment": map[string]any{"id": "c1", "url": "https://linear.app/c/c1"},
					},
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"issue": map[string]any{"id": "issue-1", "identifier": "ENG-1"},
				},
			})
		}
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
	root.SetArgs([]string{"issue", "comment", "add", "ENG-1", "--body", "Looks good"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v stderr=%s", err, errBuf)
	}
	if !strings.Contains(buf.String(), "Comment added") {
		t.Fatalf("unexpected: %s", buf.String())
	}
}

func TestIssueArchiveRequiresConfirm(t *testing.T) {
	cmd := cli.NewRoot()
	archive, _, err := cmd.Find([]string{"issue", "archive"})
	if err != nil {
		t.Fatal(err)
	}
	if archive.Flags().Lookup("confirm") == nil {
		t.Fatal("expected --confirm on issue archive")
	}
	if archive.Flags().Lookup("bulk") == nil {
		t.Fatal("expected --bulk on issue archive")
	}
}

func TestIssueArchiveWithConfirm(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query string `json:"query"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if strings.Contains(body.Query, "issueArchive") || strings.Contains(body.Query, "ArchiveIssue") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"issueArchive": map[string]any{"success": true},
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
	root.SetArgs([]string{"issue", "archive", "ENG-123", "--confirm"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v stderr=%s", err, errBuf)
	}
	if !strings.Contains(buf.String(), "archived") {
		t.Fatalf("unexpected: %s stderr=%s", buf.String(), errBuf.String())
	}
}

func TestIssueArchiveBulk(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query string `json:"query"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if strings.Contains(body.Query, "issueArchive") || strings.Contains(body.Query, "ArchiveIssue") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"issueArchive": map[string]any{"success": true},
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
	root.SetArgs([]string{"issue", "archive", "--bulk", "ENG-1", "--bulk", "ENG-2", "--confirm"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v stderr=%s", err, errBuf)
	}
	if !strings.Contains(buf.String(), "2 succeeded") {
		t.Fatalf("unexpected: %s", buf.String())
	}
}

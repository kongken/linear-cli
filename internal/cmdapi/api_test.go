package cmdapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kongken/linear-cli/internal/cli"
)

func TestAPIBasicQuery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"viewer": map[string]any{"id": "user-1", "name": "Test User"},
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
	root.SetArgs([]string{"api", "query GetViewer { viewer { id name } }"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(buf.String(), "Test User") {
		t.Fatalf("unexpected output: %s", buf.String())
	}
}

func TestAPIVariableFlag(t *testing.T) {
	var gotVars map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Variables map[string]any `json:"variables"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotVars = body.Variables
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"team": map[string]any{"name": "Backend Team"},
			},
		})
	}))
	defer srv.Close()
	t.Setenv("LINEAR_API_KEY", "k")
	t.Setenv("LINEAR_GRAPHQL_ENDPOINT", srv.URL)

	root := cli.NewRoot()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{
		"api",
		"query GetTeam($teamId: String!) { team(id: $teamId) { name } }",
		"--variable", "teamId=abc123",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if gotVars["teamId"] != "abc123" {
		t.Fatalf("variables = %#v", gotVars)
	}
	if !strings.Contains(buf.String(), "Backend Team") {
		t.Fatalf("output: %s", buf.String())
	}
}

func TestAPIVariableCoercion(t *testing.T) {
	var gotVars map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Variables map[string]any `json:"variables"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotVars = body.Variables
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{}})
	}))
	defer srv.Close()
	t.Setenv("LINEAR_API_KEY", "k")
	t.Setenv("LINEAR_GRAPHQL_ENDPOINT", srv.URL)

	root := cli.NewRoot()
	root.SetOut(new(bytes.Buffer))
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{
		"api",
		"query Q($first: Int!, $active: Boolean!) { issues(first: $first) { nodes { title } } }",
		"--variable", "first=5",
		"--variable", "active=true",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if gotVars["first"] != float64(5) && gotVars["first"] != 5 {
		// json decoder may leave numbers as float64 when re-encoded; client may send int
		if n, ok := gotVars["first"].(int); !ok || n != 5 {
			if f, ok := gotVars["first"].(float64); !ok || f != 5 {
				t.Fatalf("first = %#v", gotVars["first"])
			}
		}
	}
	if gotVars["active"] != true {
		t.Fatalf("active = %#v", gotVars["active"])
	}
}

func TestAPIHelpNamesPositional(t *testing.T) {
	root := cli.NewRoot()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"api", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "graphqlDocument") {
		t.Fatalf("help missing graphqlDocument: %s", out)
	}
}

func TestAPITooManyArgs(t *testing.T) {
	root := cli.NewRoot()
	errBuf := new(bytes.Buffer)
	root.SetOut(new(bytes.Buffer))
	root.SetErr(errBuf)
	root.SetArgs([]string{"api", "query", "query { viewer { id } }"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for too many args")
	}
}

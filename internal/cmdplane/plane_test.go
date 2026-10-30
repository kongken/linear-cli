package cmdplane_test

import (
	"bytes"
	"encoding/json"
	"github.com/kongken/linear-cli/internal/keyring"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kongken/linear-cli/internal/cmdplane"
	"github.com/kongken/linear-cli/internal/planeconfig"
)

func TestCreateConvertsMarkdownAndKeepsNativeJSON(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("PLANE_API_KEY", "test-token")
	t.Setenv("PLANE_PROFILE", "")
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description_html"`
		Priority    string `json:"priority"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/v2/workspaces/acme/projects/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa/work-items/" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"id":"created-id","name":"Ship Plane","assignee_ids":[],"custom_field":"retained"}`))
	}))
	defer server.Close()
	dir := t.TempDir()
	root := cmdplane.NewWithOptions(cmdplane.Options{Resolver: planeconfig.Resolver{UserFile: filepath.Join(dir, "config.toml"), ProjectDir: dir}})
	out := new(bytes.Buffer)
	root.SetOut(out)
	root.SetArgs([]string{"--no-profile", "--base-url", server.URL, "--allow-http", "--api-version", "v2", "--workspace", "acme", "issue", "create", "--project", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "--title", "Ship Plane", "--description", "**Markdown**", "--priority", "high", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if body.Name != "Ship Plane" || body.Description != "<p><strong>Markdown</strong></p>\n" || body.Priority != "high" {
		t.Fatalf("unexpected create payload: %+v", body)
	}
	if !json.Valid(out.Bytes()) || !strings.Contains(out.String(), `"custom_field": "retained"`) {
		t.Fatalf("lost native JSON: %s", out)
	}
}

func TestQueryUsesServerSideFiltersAndRejectsUnsupportedV1(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "test-token")
	t.Setenv("PLANE_PROFILE", "")
	for _, version := range []string{"v1", "v2"} {
		t.Run(version, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Query().Get("assignee_id") != "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb" || r.URL.Query().Get("priority") != "high" {
					t.Errorf("wrong server-side filter: %s", r.URL)
				}
				_, _ = w.Write([]byte(`{"data":[],"next":null,"pagination":{"style":"offset"}}`))
			}))
			defer server.Close()
			dir := t.TempDir()
			root := cmdplane.NewWithOptions(cmdplane.Options{Resolver: planeconfig.Resolver{UserFile: filepath.Join(dir, "config.toml"), ProjectDir: dir}})
			root.SetOut(new(bytes.Buffer))
			root.SetErr(new(bytes.Buffer))
			root.SetArgs([]string{"--no-profile", "--base-url", server.URL, "--allow-http", "--workspace", "acme", "--api-version", version, "issue", "query", "--project", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "--assignee", "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", "--priority", "high", "--json"})
			err := root.Execute()
			if version == "v1" {
				if err == nil || calls != 0 {
					t.Fatalf("v1 filter falsely accepted: calls=%d err=%v", calls, err)
				}
			} else if err != nil || calls != 1 {
				t.Fatalf("query: calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestSavedProfileSupportsUpdateAndCommentsWithoutEnvironmentToken(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "")
	t.Setenv("PLANE_PROFILE", "")
	const project = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	const user = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	const issue = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	var patched bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "saved-token" {
			t.Error("saved profile did not supply the PAT")
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v2/users/me/":
			_, _ = w.Write([]byte(`{"id":"` + user + `"}`))
		case r.URL.Path == "/api/v2/workspaces/acme/projects/":
			_, _ = w.Write([]byte(`{"data":[{"id":"` + project + `","identifier":"ENG","name":"Engineering"}],"next":null,"pagination":{"style":"offset"}}`))
		case strings.HasSuffix(r.URL.Path, "/members/"):
			if r.URL.Query().Get("expand") != "member" {
				t.Error("membership expansion missing")
			}
			_, _ = w.Write([]byte(`{"data":[{"id":"membership-not-user-id","member_id":"` + user + `","member":{"email":"alice@example.com"}}],"next":null,"pagination":{"style":"offset"}}`))
		case strings.HasSuffix(r.URL.Path, "/work-items/ENG-42/"):
			_, _ = w.Write([]byte(`{"id":"` + issue + `","project_id":"` + project + `","identifier":"ENG-42","name":"Keep title"}`))
		case r.Method == "PATCH" && strings.HasSuffix(r.URL.Path, "/work-items/"+issue+"/"):
			var body map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if string(body["assignee_ids"]) != `["`+user+`"]` || string(body["label_ids"]) != `[]` || string(body["description_html"]) != `""` {
				t.Errorf("incorrect replacement/clear payload: %s", body)
			}
			if _, ok := body["name"]; ok {
				t.Error("unprovided title was overwritten")
			}
			patched = true
			_, _ = w.Write([]byte(`{"id":"` + issue + `","name":"Keep title","assignee_ids":["` + user + `"],"label_ids":[]}`))
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/comments/"):
			var body struct {
				HTML string `json:"comment_html"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.HTML != "<p>Next step</p>\n" {
				t.Errorf("comment HTML %q", body.HTML)
			}
			w.WriteHeader(201)
			_, _ = w.Write([]byte(`{"id":"comment","comment_html":"<p>Next step</p>"}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	dir := t.TempDir()
	opts := cmdplane.Options{Resolver: planeconfig.Resolver{UserFile: filepath.Join(dir, "config.toml"), ProjectDir: dir}, Keyring: keyring.NewMemoryStore()}
	execute := func(input string, args ...string) error {
		cmd := cmdplane.NewWithOptions(opts)
		cmd.SetOut(new(bytes.Buffer))
		cmd.SetErr(new(bytes.Buffer))
		cmd.SetIn(strings.NewReader(input))
		cmd.SetArgs(args)
		return cmd.Execute()
	}
	if err := execute("saved-token\n", "auth", "login", "--profile", "cloud", "--base-url", server.URL, "--allow-http", "--workspace", "acme", "--api-version", "v2", "--project", project, "--token-stdin"); err != nil {
		t.Fatal(err)
	}
	if err := execute("", "issue", "update", "ENG-42", "--assignee", "alice@example.com", "--clear-labels", "--description", "", "--json"); err != nil {
		t.Fatal(err)
	}
	if !patched {
		t.Fatal("update was not sent")
	}
	if err := execute("", "issue", "comment", "add", "ENG-42", "--body", "Next step", "--json"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"config.toml", "credentials.toml"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte("saved-token")) {
			t.Fatalf("keyring token leaked into %s", name)
		}
	}
	if err := execute("", "auth", "logout"); err != nil {
		t.Fatal(err)
	}
	if err := execute("", "auth", "status"); err == nil {
		t.Fatal("authentication succeeds after logout")
	}
}

func TestDeleteRequiresExplicitConfirmationBeforeAnyRequest(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "token")
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(204) }))
	defer server.Close()
	dir := t.TempDir()
	root := cmdplane.NewWithOptions(cmdplane.Options{Resolver: planeconfig.Resolver{UserFile: filepath.Join(dir, "config.toml"), ProjectDir: dir}})
	root.SetOut(new(bytes.Buffer))
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"--no-profile", "--base-url", server.URL, "--allow-http", "--api-version", "v2", "--workspace", "acme", "issue", "delete", "ENG-42"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "--yes") || calls != 0 {
		t.Fatalf("missing confirmation: calls=%d err=%v", calls, err)
	}
}

func TestStartPreservesLocalBranchWhenRemoteUpdateFails(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "token")
	t.Setenv("PLANE_PROFILE", "")
	dir := t.TempDir()
	t.Chdir(dir)
	git := func(args ...string) string {
		cmd := exec.Command("git", args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s %v", args, out, err)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "--quiet")
	git("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "Initial")
	requests, writes := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "PATCH" {
			writes++
			w.WriteHeader(503)
			_, _ = w.Write([]byte(`{"detail":"temporary"}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/states/") {
			_, _ = w.Write([]byte(`{"data":[{"id":"started-state","group":"started"}],"next":null,"pagination":{"style":"offset"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"issue-id","project_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","identifier":"ENG-42"}`))
	}))
	defer server.Close()
	opts := cmdplane.Options{Resolver: planeconfig.Resolver{UserFile: filepath.Join(t.TempDir(), "config.toml"), ProjectDir: dir}}
	execute := func() error {
		cmd := cmdplane.NewWithOptions(opts)
		cmd.SetOut(new(bytes.Buffer))
		cmd.SetErr(new(bytes.Buffer))
		cmd.SetArgs([]string{"--no-profile", "--base-url", server.URL, "--allow-http", "--workspace", "acme", "--api-version", "v2", "issue", "start", "ENG-42"})
		return cmd.Execute()
	}
	if err := os.WriteFile("uncommitted.txt", []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := execute(); err == nil || requests != 0 {
		t.Fatalf("dirty worktree not rejected: requests=%d err=%v", requests, err)
	}
	if err := os.Remove("uncommitted.txt"); err != nil {
		t.Fatal(err)
	}
	before := git("rev-parse", "HEAD")
	err := execute()
	if err == nil || !strings.Contains(err.Error(), "branch eng-42 is selected") || writes != 1 {
		t.Fatalf("partial result not reported: writes=%d err=%v", writes, err)
	}
	if branch := git("branch", "--show-current"); branch != "eng-42" {
		t.Fatalf("branch %s", branch)
	}
	if after := git("rev-parse", "HEAD"); after != before {
		t.Fatalf("branch was reset: %s != %s", after, before)
	}
}

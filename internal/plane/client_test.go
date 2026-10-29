package plane_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/kongken/linear-cli/internal/plane"
	"github.com/kongken/linear-cli/internal/planeconfig"
)

func TestWorkItemRoundTripUsesVersionContract(t *testing.T) {
	for _, version := range []string{"v1", "v2"} {
		t.Run(version, func(t *testing.T) {
			var saved map[string]json.RawMessage
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-API-Key") != "plane-test" || r.Header.Get("Authorization") != "" {
					t.Error("incorrect Plane authentication")
				}
				if !strings.HasPrefix(r.URL.Path, "/prefix/api/"+version+"/workspaces/acme/") {
					t.Errorf("lost deployment prefix: %s", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodPost {
					if err := json.NewDecoder(r.Body).Decode(&saved); err != nil {
						t.Error(err)
					}
					stateKey, assigneeKey := "state_id", "assignee_ids"
					if version == "v1" {
						stateKey, assigneeKey = "state", "assignees"
					}
					if string(saved[stateKey]) != `"state-uuid"` || string(saved[assigneeKey]) != `[]` {
						t.Errorf("wrong relation payload: %v", saved)
					}
					w.WriteHeader(http.StatusCreated)
				}
				_, _ = w.Write([]byte(`{"id":"item-uuid","identifier":"ENG-42","name":"Ship Plane"}`))
			}))
			defer server.Close()
			c, err := plane.NewClient(planeconfig.Context{Profile: planeconfig.Profile{BaseURL: server.URL + "/prefix", Workspace: "acme", APIVersion: version, AllowHTTP: true}}, "plane-test", plane.ClientOptions{})
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.CreateIssue(context.Background(), "project-uuid", plane.IssueInput{Name: plane.Set("Ship Plane"), State: plane.Set(ptr("state-uuid")), Assignees: plane.Set([]string{})})
			if err != nil {
				t.Fatal(err)
			}
			raw, err := c.GetIssue(context.Background(), "", "ENG-42")
			if err != nil || !strings.Contains(string(raw), "Ship Plane") {
				t.Fatalf("get: %s %v", raw, err)
			}
		})
	}
}
func ptr(s string) *string { return &s }

func TestCursorPagesAreTraversedWithoutChangingFilters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("search") != "bug" {
			t.Error("lost search filter")
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("cursor") == "" {
			_, _ = w.Write([]byte(`{"data":[{"id":"first"}],"pagination":{"style":"cursor"},"next_cursor":"next-token","has_more":true}`))
		} else {
			_, _ = w.Write([]byte(`{"data":[{"id":"second"}],"pagination":{"style":"cursor"},"next_cursor":null,"has_more":false}`))
		}
	}))
	defer server.Close()
	c, err := plane.NewClient(planeconfig.Context{Profile: planeconfig.Profile{BaseURL: server.URL, Workspace: "acme", APIVersion: "v2", AllowHTTP: true}}, "token", plane.ClientOptions{})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := c.Collect(context.Background(), "work-items", "project", url.Values{"search": {"bug"}, "paginate": {"cursor"}}, 10)
	if err != nil || len(rows) != 2 {
		t.Fatalf("collect %s %v", rows, err)
	}
}

func TestWritesAreNotRetriedAndErrorDoesNotLeakToken(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(503)
		_, _ = w.Write([]byte(`{"detail":"failure with private-token"}`))
	}))
	defer server.Close()
	c, err := plane.NewClient(planeconfig.Context{Profile: planeconfig.Profile{BaseURL: server.URL, Workspace: "acme", APIVersion: "v2", AllowHTTP: true}}, "private-token", plane.ClientOptions{Wait: func(context.Context, time.Duration) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.CreateIssue(context.Background(), "project", plane.IssueInput{Name: plane.Set("No duplicate")})
	if err == nil || calls != 1 || strings.Contains(err.Error(), "private-token") {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestRawAPIRejectsCredentialExfiltrationAndRedirects(t *testing.T) {
	leaked := false
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked = true; _, _ = w.Write([]byte(`{}`)) }))
	defer other.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/api/v2/users/me/", http.StatusFound)
	}))
	defer server.Close()
	c, err := plane.NewClient(planeconfig.Context{Profile: planeconfig.Profile{BaseURL: server.URL, Workspace: "acme", APIVersion: "v2", AllowHTTP: true}}, "token", plane.ClientOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{other.URL + "/api/v2/users/me/", "//another.example/api/v2/users/me/", "/api/v1/users/me/", "/api/v2/%2e%2e/secret"} {
		if _, err := c.RequestRaw(context.Background(), "GET", path, nil); err == nil {
			t.Errorf("accepted unsafe path %s", path)
		}
	}
	if _, err := c.CurrentUser(context.Background()); err == nil {
		t.Error("redirect reported success")
	}
	if leaked {
		t.Fatal("Plane token followed a redirect")
	}
}

func TestWriteCancellationReportsUnknownResult(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		cancel()
		if err != nil {
			t.Error(err)
			return
		}
		_ = conn.Close()
	}))
	defer server.Close()
	c, err := plane.NewClient(planeconfig.Context{Profile: planeconfig.Profile{BaseURL: server.URL, Workspace: "acme", APIVersion: "v2", AllowHTTP: true}}, "token", plane.ClientOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.CreateIssue(ctx, "project", plane.IssueInput{Name: plane.Set("once")})
	if err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("write cancellation lost its uncertain result: %v", err)
	}
}

func TestCycleMembershipUsesTheSelectedVersion(t *testing.T) {
	for _, version := range []string{"v1", "v2"} {
		t.Run(version, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]json.RawMessage
				if r.Method == "POST" {
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
				}
				if version == "v1" {
					if !strings.Contains(r.URL.Path, "/cycles/cycle/cycle-issues/") {
						t.Errorf("wrong v1 route %s", r.URL)
					}
					if r.Method == "POST" && string(body["issues"]) != `["issue"]` {
						t.Errorf("wrong v1 body %s", body)
					}
					if r.Method == "DELETE" && !strings.HasSuffix(r.URL.Path, "/issue/") {
						t.Errorf("wrong removal route %s", r.URL)
					}
				} else {
					if r.Method != "POST" || !strings.HasSuffix(r.URL.Path, "/cycles/cycle/work-items/") {
						t.Errorf("wrong v2 route %s %s", r.Method, r.URL)
					}
					if string(body["add"]) != `["issue"]` && string(body["remove"]) != `["issue"]` {
						t.Errorf("wrong v2 body %s", body)
					}
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()
			c, err := plane.NewClient(planeconfig.Context{Profile: planeconfig.Profile{BaseURL: server.URL, Workspace: "acme", APIVersion: version, AllowHTTP: true}}, "token", plane.ClientOptions{})
			if err != nil {
				t.Fatal(err)
			}
			for _, add := range []bool{true, false} {
				if _, err := c.Membership(context.Background(), "cycles", "project", "cycle", "issue", add); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestReadHonorsRetryAfterWithoutSleepingInTest(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "3")
			w.WriteHeader(429)
			_, _ = w.Write([]byte(`{"code":"rate_limited","detail":"wait"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"user"}`))
	}))
	defer server.Close()
	var waits []time.Duration
	c, err := plane.NewClient(planeconfig.Context{Profile: planeconfig.Profile{BaseURL: server.URL, Workspace: "acme", APIVersion: "v2", AllowHTTP: true}}, "token", plane.ClientOptions{Wait: func(ctx context.Context, d time.Duration) error { waits = append(waits, d); return nil }})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := c.CurrentUser(context.Background())
	if err != nil || !strings.Contains(string(raw), "user") || len(waits) != 1 || waits[0] != 3*time.Second || calls != 2 {
		t.Fatalf("calls=%d waits=%v raw=%s err=%v", calls, waits, raw, err)
	}
}

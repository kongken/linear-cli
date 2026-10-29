package workcli_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kongken/linear-cli/internal/cli"
	"github.com/kongken/linear-cli/internal/workcli"
)

func TestLinearJSONCompatibility(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LINEAR_API_KEY", "linear-test")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "linear-test" {
			t.Error("Linear credential changed")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"issue":{"identifier":"ENG-42","title":"Keep Linear JSON","labels":{"nodes":[]}}}}`))
	}))
	defer server.Close()
	t.Setenv("LINEAR_GRAPHQL_ENDPOINT", server.URL)
	oldOutput, newOutput := new(bytes.Buffer), new(bytes.Buffer)
	old := cli.NewRoot()
	old.SetOut(oldOutput)
	old.SetArgs([]string{"issue", "view", "ENG-42", "--json"})
	if err := old.Execute(); err != nil {
		t.Fatal(err)
	}
	root := workcli.NewRoot()
	root.SetOut(newOutput)
	root.SetArgs([]string{"linear", "issue", "view", "ENG-42", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if oldOutput.String() != newOutput.String() {
		t.Fatalf("Linear output changed:\nold %s\nnew %s", oldOutput, newOutput)
	}
}

func TestPlaneHelpRequiresNoCredentials(t *testing.T) {
	root := workcli.NewRoot()
	output := new(bytes.Buffer)
	root.SetOut(output)
	root.SetArgs([]string{"plane", "issue", "create", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "--assignee") {
		t.Fatalf("missing Plane creation flags: %s", output)
	}
}

func TestPlaneArgumentErrorsHaveNoDataOutput(t *testing.T) {
	for _, args := range [][]string{{"plane", "issue", "not-a-command"}, {"plane", "issue", "create"}, {"plane", "--unknown-option"}} {
		out, errOut := new(bytes.Buffer), new(bytes.Buffer)
		code := workcli.Run(context.Background(), args, strings.NewReader(""), out, errOut)
		if code != 2 || out.Len() != 0 || errOut.Len() == 0 {
			t.Fatalf("args=%v code=%d stdout=%q stderr=%q", args, code, out, errOut)
		}
		if strings.Count(errOut.String(), "✗") != 1 {
			t.Fatalf("error rendered more than once: %s", errOut)
		}
	}
}

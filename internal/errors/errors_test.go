package errors_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/kongken/linear-cli/internal/errors"
)

func TestIsDebugMode(t *testing.T) {
	t.Setenv("LINEAR_DEBUG", "")
	os.Unsetenv("LINEAR_DEBUG")
	if errors.IsDebugMode() {
		t.Fatal("expected false when unset")
	}
	t.Setenv("LINEAR_DEBUG", "1")
	if !errors.IsDebugMode() {
		t.Fatal("expected true for 1")
	}
	t.Setenv("LINEAR_DEBUG", "true")
	if !errors.IsDebugMode() {
		t.Fatal("expected true for true")
	}
}

func TestNotFoundErrorMessage(t *testing.T) {
	err := errors.NewNotFoundError("Issue", "ENG-123")
	if err.UserMessage != "Issue not found: ENG-123" {
		t.Fatalf("got %q", err.UserMessage)
	}
	if err.EntityType != "Issue" || err.Identifier != "ENG-123" {
		t.Fatalf("fields: %+v", err)
	}
}

func TestValidationErrorSuggestion(t *testing.T) {
	err := errors.NewValidationError("bad", errors.WithSuggestion("try x"))
	if err.UserMessage != "bad" || err.Suggestion != "try x" {
		t.Fatalf("%+v", err)
	}
}

func TestAuthErrorDefaultSuggestion(t *testing.T) {
	err := errors.NewAuthError("no key")
	if !strings.Contains(err.Suggestion, "linear auth login") {
		t.Fatalf("suggestion: %q", err.Suggestion)
	}
}

func TestHandleErrorWritesPrefix(t *testing.T) {
	buf := new(bytes.Buffer)
	errors.HandleErrorTo(buf, errors.NewValidationError("bad", errors.WithSuggestion("try x")), "Failed to foo")
	out := buf.String()
	if !strings.Contains(out, "✗") {
		t.Fatalf("missing ✗: %q", out)
	}
	if !strings.Contains(out, "Failed to foo: bad") {
		t.Fatalf("missing context/message: %q", out)
	}
	if !strings.Contains(out, "try x") {
		t.Fatalf("missing suggestion: %q", out)
	}
}

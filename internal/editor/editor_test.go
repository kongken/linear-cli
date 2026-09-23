package editor_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kongken/linear-cli/internal/editor"
)

func TestGetEditorFromEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "missing-gitconfig"))
	t.Setenv("GIT_CONFIG_SYSTEM", filepath.Join(home, "missing-system"))
	t.Setenv("EDITOR", "my-custom-editor")

	got := editor.Get()
	if got != "my-custom-editor" {
		t.Fatalf("Get() = %q, want my-custom-editor", got)
	}
}

func TestGetEditorEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "missing-gitconfig"))
	t.Setenv("GIT_CONFIG_SYSTEM", filepath.Join(home, "missing-system"))
	t.Setenv("EDITOR", "")

	got := editor.Get()
	if got != "" {
		t.Fatalf("Get() = %q, want empty", got)
	}
}

func TestOpenReadsEditorOutput(t *testing.T) {
	script := writeEditorScript(t, "edited body\n")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "missing"))
	t.Setenv("EDITOR", script)

	got, err := editor.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got != "edited body" {
		t.Fatalf("Open() = %q, want edited body", got)
	}
}

func TestOpenWithContent(t *testing.T) {
	script := writeEditorScript(t, "updated\n")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "missing"))
	t.Setenv("EDITOR", script)

	got, err := editor.OpenWithContent("initial")
	if err != nil {
		t.Fatalf("OpenWithContent: %v", err)
	}
	if got != "updated" {
		t.Fatalf("got %q", got)
	}
}

func TestOpenEmptyFileReturnsEmpty(t *testing.T) {
	script := writeEditorScript(t, "   \n")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "missing"))
	t.Setenv("EDITOR", script)

	got, err := editor.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got != "" {
		t.Fatalf("Open() = %q, want empty", got)
	}
}

func TestOpenNoEditor(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "missing"))
	t.Setenv("EDITOR", "")

	_, err := editor.Open()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "No editor found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func writeEditorScript(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fake-editor")
	if runtime.GOOS == "windows" {
		path += ".bat"
		body := "@echo off\necho " + strings.TrimSpace(content) + " > %1\n"
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	body := "#!/bin/sh\ncat > \"$1\" <<'EOF'\n" + content + "EOF\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	// Ensure executable bit is honored on some filesystems
	if err := exec.Command("chmod", "+x", path).Run(); err != nil {
		t.Fatal(err)
	}
	return path
}

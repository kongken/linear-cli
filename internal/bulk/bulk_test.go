package bulk_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kongken/linear-cli/internal/bulk"
)

func TestParseIDs(t *testing.T) {
	got := bulk.ParseIDs("ENG-1\nENG-2, ENG-3  ENG-4")
	if len(got) != 4 {
		t.Fatalf("got %#v", got)
	}
	if got[0] != "ENG-1" || got[3] != "ENG-4" {
		t.Fatalf("%#v", got)
	}
}

func TestCollectFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ids.txt")
	if err := os.WriteFile(path, []byte("ENG-1\nENG-2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ids, err := bulk.Collect(bulk.Sources{File: path})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(ids, ",") != "ENG-1,ENG-2" {
		t.Fatalf("%#v", ids)
	}
}

func TestCollectRejectsMixedPositional(t *testing.T) {
	_, err := bulk.Collect(bulk.Sources{Bulk: []string{"ENG-1"}, Positional: "ENG-2"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestIsBulkMode(t *testing.T) {
	if !bulk.IsMode(bulk.Sources{Bulk: []string{"a"}}) {
		t.Fatal("expected bulk mode")
	}
	if bulk.IsMode(bulk.Sources{}) {
		t.Fatal("expected non-bulk")
	}
}

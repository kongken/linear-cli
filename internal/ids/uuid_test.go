package ids_test

import (
	"testing"

	"github.com/kongken/linear-cli/internal/ids"
)

func TestIsUUID(t *testing.T) {
	if !ids.IsUUID("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee") {
		t.Fatal("expected uuid")
	}
	if ids.IsUUID("Alpha") || ids.IsUUID("ENG-123") || ids.IsUUID("") {
		t.Fatal("expected non-uuid")
	}
}

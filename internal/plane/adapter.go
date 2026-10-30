package plane

import (
	"encoding/json"
	"fmt"
	clierrors "github.com/kongken/linear-cli/internal/errors"
	"net/url"
)

// adapter is the internal seam where v1/v2 wire formats vary. Commands use the
// same typed operations and never build version-specific issue payloads.
type adapter interface {
	issueBody(IssueInput) (json.RawMessage, error)
	page(json.RawMessage, url.Values) (Page, error)
}

func decodeItems(raw json.RawMessage) ([]json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, fmt.Errorf("invalid list response: missing resource array")
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("invalid resource array")
	}
	return items, nil
}
func cursorQuery(query url.Values, cursor *string) (url.Values, error) {
	if cursor == nil || *cursor == "" || *cursor == query.Get("cursor") {
		return nil, fmt.Errorf("invalid pagination: missing or repeated cursor")
	}
	next := cloneQuery(query)
	next.Set("cursor", *cursor)
	return next, nil
}

func badInput(message string) error { return clierrors.NewValidationError(message) }

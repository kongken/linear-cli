package plane

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// Directory is complete or returns an error. Name resolution must not infer
// uniqueness from a truncated list. Cache entries live only for this client.
func (c *Client) Directory(ctx context.Context, resource, project string) ([]json.RawMessage, error) {
	path, err := c.ResourcePath(resource, project)
	if err != nil {
		return nil, err
	}
	c.directoryMu.Lock()
	cached, ok := c.directories[path]
	c.directoryMu.Unlock()
	if ok {
		return copyRows(cached), nil
	}
	query := url.Values{"per_page": {"100"}}
	if resource == "members" && c.Context.APIVersion == "v2" {
		query.Set("expand", "member")
	}
	rows, err := c.collectPath(ctx, path, query, 10000, true)
	if err != nil {
		return nil, err
	}
	c.directoryMu.Lock()
	c.directories[path] = copyRows(rows)
	c.directoryMu.Unlock()
	return rows, nil
}
func copyRows(rows []json.RawMessage) []json.RawMessage {
	copy := make([]json.RawMessage, len(rows))
	for i, row := range rows {
		copy[i] = append(json.RawMessage(nil), row...)
	}
	return copy
}
func (c *Client) invalidateDirectories() {
	c.directoryMu.Lock()
	c.directories = map[string][]json.RawMessage{}
	c.directoryMu.Unlock()
}

func directoryLimitError() error {
	return fmt.Errorf("directory exceeds 10000 resources; use explicit UUIDs instead of name resolution")
}

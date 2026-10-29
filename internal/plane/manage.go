package plane

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// MutateResource accepts an explicit version-native JSON body for directory administration.
func (c *Client) MutateResource(ctx context.Context, resource, project, id, method string, body json.RawMessage) (json.RawMessage, error) {
	path, err := c.ResourcePath(resource, project)
	if err != nil {
		return nil, err
	}
	switch method {
	case http.MethodPost:
		if id != "" {
			return nil, fmt.Errorf("create does not take an existing resource ID")
		}
	case http.MethodPatch, http.MethodDelete:
		if err := segment(id); err != nil {
			return nil, err
		}
		path += url.PathEscape(id) + "/"
	default:
		return nil, fmt.Errorf("unsupported resource mutation")
	}
	return c.RequestRaw(ctx, method, path, body)
}
func (c *Client) IssueAction(ctx context.Context, ref IssueRef, action string) (json.RawMessage, error) {
	path, err := c.ResourcePath("work-items", ref.ProjectID)
	if err != nil {
		return nil, err
	}
	if err := segment(ref.ID); err != nil {
		return nil, err
	}
	path += url.PathEscape(ref.ID) + "/"
	switch action {
	case "delete":
		return c.RequestRaw(ctx, http.MethodDelete, path, nil)
	case "archive", "unarchive":
		if c.Context.APIVersion != "v2" {
			return nil, fmt.Errorf("archive actions are not verified for v1; use an explicit version-native API request")
		}
		return c.RequestRaw(ctx, http.MethodPost, path+action+"/", nil)
	default:
		return nil, fmt.Errorf("unsupported work-item action")
	}
}
func (c *Client) SubresourcePath(ref IssueRef, resource string) (string, error) {
	switch resource {
	case "comments", "links", "attachments":
	default:
		return "", fmt.Errorf("unsupported work-item subresource")
	}
	path, err := c.ResourcePath("work-items", ref.ProjectID)
	if err != nil {
		return "", err
	}
	if err := segment(ref.ID); err != nil {
		return "", err
	}
	return path + url.PathEscape(ref.ID) + "/" + resource + "/", nil
}
func (c *Client) MutateSubresource(ctx context.Context, ref IssueRef, resource, id, method string, body json.RawMessage) (json.RawMessage, error) {
	path, err := c.SubresourcePath(ref, resource)
	if err != nil {
		return nil, err
	}
	switch method {
	case http.MethodPost:
		if id != "" {
			return nil, fmt.Errorf("create does not accept an existing resource ID")
		}
	case http.MethodPatch, http.MethodDelete:
		if err := segment(id); err != nil {
			return nil, err
		}
		path += url.PathEscape(id) + "/"
	default:
		return nil, fmt.Errorf("unsupported subresource mutation")
	}
	return c.RequestRaw(ctx, method, path, body)
}
func (c *Client) Membership(ctx context.Context, resource, project, containerID, issueID string, add bool) (json.RawMessage, error) {
	if resource != "cycles" && resource != "modules" {
		return nil, fmt.Errorf("membership requires a cycle or module")
	}
	if err := segment(containerID); err != nil {
		return nil, err
	}
	if err := segment(issueID); err != nil {
		return nil, err
	}
	path, err := c.ResourcePath(resource, project)
	if err != nil {
		return nil, err
	}
	path += url.PathEscape(containerID) + "/"
	var body json.RawMessage
	method := http.MethodPost
	if c.Context.APIVersion == "v1" {
		part := "cycle-issues"
		if resource == "modules" {
			part = "module-issues"
		}
		path += part + "/"
		if add {
			body, err = json.Marshal(struct {
				Issues []string `json:"issues"`
			}{[]string{issueID}})
		} else {
			path += url.PathEscape(issueID) + "/"
			method = http.MethodDelete
		}
	} else {
		path += "work-items/"
		change := struct {
			Add    []string `json:"add"`
			Remove []string `json:"remove"`
		}{Add: []string{}, Remove: []string{}}
		if add {
			change.Add = []string{issueID}
		} else {
			change.Remove = []string{issueID}
		}
		body, err = json.Marshal(change)
	}
	if err != nil {
		return nil, err
	}
	return c.RequestRaw(ctx, method, path, body)
}

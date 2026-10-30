package plane

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

type Field[T interface{}] struct {
	Set   bool
	Value T
}

func Set[T interface{}](value T) Field[T] { return Field[T]{Set: true, Value: value} }

type IssueInput struct {
	Name            Field[string]
	DescriptionHTML Field[string]
	Priority        Field[string]
	State           Field[*string]
	Assignees       Field[[]string]
	Labels          Field[[]string]
	Parent          Field[*string]
	TargetDate      Field[*string]
	Estimate        Field[*string]
}

func put[T interface{}](body map[string]json.RawMessage, key string, f Field[T]) error {
	if !f.Set {
		return nil
	}
	raw, err := json.Marshal(f.Value)
	if err != nil {
		return err
	}
	body[key] = raw
	return nil
}
func (in IssueInput) payload(keys []string) (json.RawMessage, error) {
	body := map[string]json.RawMessage{}
	if in.Priority.Set {
		switch in.Priority.Value {
		case "none", "urgent", "high", "medium", "low":
		default:
			return nil, badInput("priority must be none, urgent, high, medium or low")
		}
	}
	for _, err := range []error{
		put(body, "name", in.Name), put(body, "description_html", in.DescriptionHTML), put(body, "priority", in.Priority),
		put(body, keys[0], in.State), put(body, keys[1], in.Assignees), put(body, keys[2], in.Labels), put(body, keys[3], in.Parent), put(body, keys[4], in.Estimate), put(body, "target_date", in.TargetDate),
	} {
		if err != nil {
			return nil, err
		}
	}
	if len(body) == 0 {
		return nil, badInput("at least one update field is required")
	}
	return json.Marshal(body)
}

func (c *Client) ResourcePath(resource, project string) (string, error) {
	switch resource {
	case "projects":
		return c.workspacePath() + "projects/", nil
	case "members":
		if project == "" {
			return c.workspacePath() + "members/", nil
		}
	case "work-items":
		if project == "" {
			if c.Context.APIVersion == "v1" {
				return "", fmt.Errorf("v1 work-item lists require a project")
			}
			return c.workspacePath() + "work-items/", nil
		}
	case "states", "labels", "cycles", "modules":
	case "estimates":
		if c.Context.APIVersion == "v1" {
			return "", fmt.Errorf("v1 does not expose an estimates collection; use the configured estimate API with plane api")
		}
	default:
		return "", fmt.Errorf("unsupported Plane collection %q", resource)
	}
	p, err := c.projectPath(project)
	if err != nil {
		return "", err
	}
	return p + resource + "/", nil
}
func (c *Client) GetResource(ctx context.Context, resource, project, id string) (json.RawMessage, error) {
	if err := segment(id); err != nil {
		return nil, err
	}
	path, err := c.ResourcePath(resource, project)
	if err != nil {
		return nil, err
	}
	return c.RequestRaw(ctx, http.MethodGet, path+url.PathEscape(id)+"/", nil)
}
func (c *Client) CreateIssue(ctx context.Context, project string, in IssueInput) (json.RawMessage, error) {
	if !in.Name.Set || strings.TrimSpace(in.Name.Value) == "" {
		return nil, badInput("title is required")
	}
	path, err := c.ResourcePath("work-items", project)
	if err != nil {
		return nil, err
	}
	if project == "" {
		return nil, badInput("project is required for creation")
	}
	body, err := c.wire.issueBody(in)
	if err != nil {
		return nil, err
	}
	return c.RequestRaw(ctx, http.MethodPost, path, body)
}

var identifier = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*-[1-9][0-9]*$`)
var number = regexp.MustCompile(`^[1-9][0-9]*$`)

func (c *Client) GetIssue(ctx context.Context, project, ref string) (json.RawMessage, error) {
	if number.MatchString(ref) {
		if project == "" {
			return nil, badInput("a numeric issue reference requires --project")
		}
		raw, err := c.GetResource(ctx, "projects", "", project)
		if err != nil {
			return nil, err
		}
		var p struct {
			Identifier string `json:"identifier"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, err
		}
		if p.Identifier == "" {
			return nil, fmt.Errorf("project response is missing its identifier")
		}
		ref = p.Identifier + "-" + ref
	}
	var raw json.RawMessage
	var err error
	if identifier.MatchString(ref) {
		ref = strings.ToUpper(ref)
		raw, err = c.RequestRaw(ctx, http.MethodGet, c.workspacePath()+"work-items/"+url.PathEscape(ref)+"/", nil)
	} else {
		if _, err := uuid.Parse(ref); err != nil {
			return nil, badInput("issue reference must be a UUID, PROJECT-123 or a project-scoped number")
		}
		if project == "" {
			return nil, badInput("a UUID issue reference requires --project")
		}
		raw, err = c.GetResource(ctx, "work-items", project, ref)
	}
	if err != nil {
		return nil, err
	}
	var item struct {
		ID        string          `json:"id"`
		ProjectID string          `json:"project_id"`
		Project   json.RawMessage `json:"project"`
	}
	if err := json.Unmarshal(raw, &item); err != nil || item.ID == "" {
		return nil, fmt.Errorf("invalid issue response: missing ID")
	}
	actual := item.ProjectID
	if actual == "" && len(item.Project) > 0 {
		actual = projectIdentity(item.Project)
	}
	if project != "" && actual != "" && actual != project {
		return nil, badInput("issue belongs to another project")
	}
	if project != "" && identifier.MatchString(ref) && actual == "" {
		return nil, fmt.Errorf("issue response is missing the project ID needed to verify context")
	}

	return raw, nil
}

type IssueRef struct {
	ID        string
	ProjectID string
	Raw       json.RawMessage
}

func (c *Client) ResolveIssue(ctx context.Context, project, ref string) (IssueRef, error) {
	raw, err := c.GetIssue(ctx, project, ref)
	if err != nil {
		return IssueRef{}, err
	}
	var item struct {
		ID        string          `json:"id"`
		ProjectID string          `json:"project_id"`
		Project   json.RawMessage `json:"project"`
	}
	if err := json.Unmarshal(raw, &item); err != nil || item.ID == "" {
		return IssueRef{}, fmt.Errorf("invalid issue response: missing ID")
	}
	project = firstValue(item.ProjectID, projectIdentity(item.Project), project)
	if err := segment(project); err != nil {
		return IssueRef{}, fmt.Errorf("issue response is missing its project ID; provide --project")
	}
	return IssueRef{ID: item.ID, ProjectID: project, Raw: raw}, nil
}
func firstValue(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
func (c *Client) UpdateIssue(ctx context.Context, ref IssueRef, in IssueInput) (json.RawMessage, error) {
	path, err := c.ResourcePath("work-items", ref.ProjectID)
	if err != nil {
		return nil, err
	}
	if err := segment(ref.ID); err != nil {
		return nil, err
	}
	body, err := c.wire.issueBody(in)
	if err != nil {
		return nil, err
	}
	return c.RequestRaw(ctx, http.MethodPatch, path+url.PathEscape(ref.ID)+"/", body)
}
func (c *Client) CommentsPath(ref IssueRef) (string, error) {
	path, err := c.ResourcePath("work-items", ref.ProjectID)
	if err != nil {
		return "", err
	}
	if err := segment(ref.ID); err != nil {
		return "", err
	}
	return path + url.PathEscape(ref.ID) + "/comments/", nil
}
func (c *Client) AddComment(ctx context.Context, ref IssueRef, html, parent string) (json.RawMessage, error) {
	path, err := c.CommentsPath(ref)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(struct {
		HTML   string `json:"comment_html"`
		Parent string `json:"parent_id,omitempty"`
	}{html, parent})
	if err != nil {
		return nil, err
	}
	return c.RequestRaw(ctx, http.MethodPost, path, body)
}

type Page struct {
	Raw   json.RawMessage
	Items []json.RawMessage
	Next  url.Values
}

func cloneQuery(q url.Values) url.Values {
	out := url.Values{}
	for k, vs := range q {
		out[k] = append([]string(nil), vs...)
	}
	return out
}
func (c *Client) List(ctx context.Context, resource, project string, q url.Values) (Page, error) {
	path, err := c.ResourcePath(resource, project)
	if err != nil {
		return Page{}, err
	}
	return c.ListPath(ctx, path, q)
}
func (c *Client) ListPath(ctx context.Context, path string, q url.Values) (Page, error) {
	raw, err := c.RequestRaw(ctx, http.MethodGet, path+"?"+q.Encode(), nil)
	if err != nil {
		return Page{}, err
	}
	page := Page{Raw: raw}
	raw = bytes.TrimSpace(raw)
	if len(raw) > 0 && raw[0] == '[' {
		if err := json.Unmarshal(raw, &page.Items); err != nil {
			return page, err
		}
		return page, nil
	}
	decoded, err := c.wire.page(raw, q)
	if err != nil {
		return page, err
	}
	page.Items, page.Next = decoded.Items, decoded.Next

	if page.Next != nil && len(page.Items) == 0 {
		return page, fmt.Errorf("invalid pagination: empty page has a continuation")
	}
	return page, nil
}
func (c *Client) Collect(ctx context.Context, resource, project string, q url.Values, limit int) ([]json.RawMessage, error) {
	path, err := c.ResourcePath(resource, project)
	if err != nil {
		return nil, err
	}
	return c.CollectPath(ctx, path, q, limit)
}
func (c *Client) CollectPath(ctx context.Context, path string, q url.Values, limit int) ([]json.RawMessage, error) {
	return c.collectPath(ctx, path, q, limit, false)
}
func (c *Client) collectPath(ctx context.Context, path string, q url.Values, limit int, strict bool) ([]json.RawMessage, error) {
	if limit < 1 || limit > 10000 {
		return nil, fmt.Errorf("limit must be between 1 and 10000")
	}
	var rows []json.RawMessage
	seen := map[string]bool{}
	q = cloneQuery(q)
	for {
		key := q.Encode()
		if seen[key] {
			return nil, fmt.Errorf("pagination repeated a page")
		}
		seen[key] = true
		page, err := c.ListPath(ctx, path, q)
		if err != nil {
			return nil, err
		}
		rows = append(rows, page.Items...)
		if len(rows) >= limit {
			if strict && (page.Next != nil || len(rows) > limit) {
				return nil, directoryLimitError()
			}
			return rows[:limit], nil
		}
		if page.Next == nil {
			return rows, nil
		}
		q = page.Next
	}
}

func projectIdentity(raw json.RawMessage) string {
	var id string
	if json.Unmarshal(raw, &id) == nil {
		return id
	}
	var object struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &object) == nil {
		return object.ID
	}
	return ""
}

package issues

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/kongken/linear-cli/internal/ids"
	"github.com/kongken/linear-cli/internal/errors"
	"github.com/kongken/linear-cli/internal/gql"
	"github.com/kongken/linear-cli/internal/graphql"
	"github.com/kongken/linear-cli/internal/teams"
	"github.com/kongken/linear-cli/internal/templates"
)
// CreateInput is a non-interactive issue create payload.
type CreateInput struct {
	Title              string
	TeamKey            string
	Description        string
	Priority           *float64
	Estimate           *float64
	DueDate            string
	Assignee           string // "self", "@me", username/email, or empty
	AssigneeMe         bool   // legacy; treated as Assignee == "self"
	Parent             string
	Project            string
	State              string
	Milestone          string
	Cycle              string
	Labels             []string
	Template           string // name or UUID; when set, skips useDefaultTemplate
	NoUseDefaultTemplate bool
}

// CreatedIssue is returned after a successful create.
type CreatedIssue struct {
	ID         string `json:"id"`
	Identifier string `json:"identifier"`
	URL        string `json:"url"`
	Title      string `json:"title"`
	Team       struct {
		Key string `json:"key"`
	} `json:"team"`
}

// CreateIssue creates an issue (non-interactive path).
func CreateIssue(ctx context.Context, in CreateInput) (*CreatedIssue, error) {
	if in.Title == "" && in.Template == "" {
		return nil, errors.NewValidationError(
			"title is required",
			errors.WithSuggestion("Pass --title, --template, or run interactively in a terminal."),
		)
	}
	team, err := teams.Resolve(ctx, in.TeamKey)
	if err != nil {
		return nil, err
	}
	client, err := graphql.NewClient()
	if err != nil {
		return nil, err
	}

	input := map[string]any{
		"teamId": team.ID,
	}
	if in.Title != "" {
		input["title"] = in.Title
	}
	if in.Description != "" {
		input["description"] = in.Description
	}
	if in.Priority != nil {
		input["priority"] = *in.Priority
	}
	if in.Estimate != nil {
		input["estimate"] = *in.Estimate
	}
	if in.DueDate != "" {
		input["dueDate"] = in.DueDate
	}

	assignee := in.Assignee
	if in.AssigneeMe && assignee == "" {
		assignee = "self"
	}
	if assignee != "" {
		id, err := ResolveUserID(ctx, client, assignee)
		if err != nil {
			return nil, err
		}
		input["assigneeId"] = id
	}

	if in.Parent != "" {
		parentID, err := ResolveIssueUUID(ctx, client, in.Parent)
		if err != nil {
			return nil, err
		}
		input["parentId"] = parentID
	}

	var projectID string
	if in.Project != "" {
		projectID, err = ResolveProjectID(ctx, client, in.Project)
		if err != nil {
			return nil, err
		}
		input["projectId"] = projectID
	}

	if in.State != "" {
		stateID, err := ResolveWorkflowStateID(ctx, client, team.ID, in.State)
		if err != nil {
			return nil, err
		}
		input["stateId"] = stateID
	}

	if in.Milestone != "" {
		mid, err := ResolveMilestoneID(ctx, client, in.Milestone, projectID)
		if err != nil {
			return nil, err
		}
		input["projectMilestoneId"] = mid
	}

	if in.Cycle != "" {
		cid, err := ResolveCycleID(ctx, client, team.ID, in.Cycle)
		if err != nil {
			return nil, err
		}
		input["cycleId"] = cid
	}

	if len(in.Labels) > 0 {
		labelIDs, err := ResolveLabelIDs(ctx, client, team.Key, in.Labels)
		if err != nil {
			return nil, err
		}
		input["labelIds"] = labelIDs
	}

	if in.Template != "" {
		tmpl, err := templates.Resolve(ctx, in.Template, &templates.Scope{
			Type:    "issue",
			TeamIDs: []string{team.ID},
		})
		if err != nil {
			return nil, err
		}
		input["templateId"] = tmpl.ID
	} else if !in.NoUseDefaultTemplate {
		input["useDefaultTemplate"] = true
	}

	gqlInput := gql.IssueCreateInput{TeamId: team.ID}
	if title, ok := input["title"].(string); ok {
		gqlInput.Title = &title
	}
	if desc, ok := input["description"].(string); ok {
		gqlInput.Description = &desc
	}
	if v, ok := input["priority"].(float64); ok {
		p := int(v)
		gqlInput.Priority = &p
	}
	if v, ok := input["estimate"].(float64); ok {
		e := int(v)
		gqlInput.Estimate = &e
	}
	if v, ok := input["dueDate"].(string); ok {
		gqlInput.DueDate = &v
	}
	if v, ok := input["assigneeId"].(string); ok {
		gqlInput.AssigneeId = &v
	}
	if v, ok := input["parentId"].(string); ok {
		gqlInput.ParentId = &v
	}
	if v, ok := input["projectId"].(string); ok {
		gqlInput.ProjectId = &v
	}
	if v, ok := input["stateId"].(string); ok {
		gqlInput.StateId = &v
	}
	if v, ok := input["projectMilestoneId"].(string); ok {
		gqlInput.ProjectMilestoneId = &v
	}
	if v, ok := input["cycleId"].(string); ok {
		gqlInput.CycleId = &v
	}
	if v, ok := input["labelIds"].([]string); ok {
		gqlInput.LabelIds = v
	}
	if v, ok := input["templateId"].(string); ok {
		gqlInput.TemplateId = &v
	}
	if v, ok := input["useDefaultTemplate"].(bool); ok {
		gqlInput.UseDefaultTemplate = &v
	}

	resp, err := gql.CreateIssue(ctx, client, gqlInput)
	if err != nil {
		return nil, err
	}
	if !resp.IssueCreate.Success || resp.IssueCreate.Issue.Id == "" {
		return nil, errors.NewCliError("Issue creation failed")
	}
	issue := &CreatedIssue{
		ID:         resp.IssueCreate.Issue.Id,
		Identifier: resp.IssueCreate.Issue.Identifier,
		URL:        resp.IssueCreate.Issue.Url,
		Title:      resp.IssueCreate.Issue.Title,
	}
	issue.Team.Key = resp.IssueCreate.Issue.Team.Key
	return issue, nil
}

// ResolveUserID resolves self/@me or a user display name/email to a UUID.
func ResolveUserID(ctx context.Context, client *graphql.Client, ref string) (string, error) {
	if ref == "self" || ref == "@me" || strings.EqualFold(ref, "me") {
		return viewerID(ctx, client)
	}
	data, err := client.RequestRaw(ctx, `
query LookupUser($term: String!) {
  users(filter: { or: [
    { displayName: { eqIgnoreCase: $term } }
    { name: { eqIgnoreCase: $term } }
    { email: { eqIgnoreCase: $term } }
  ]}, first: 5) { nodes { id } }
}`, map[string]any{"term": ref})
	if err != nil {
		return "", err
	}
	var parsed struct {
		Users struct {
			Nodes []struct {
				ID string `json:"id"`
			} `json:"nodes"`
		} `json:"users"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Users.Nodes) == 0 {
		return "", errors.NewNotFoundError("User", ref)
	}
	return parsed.Users.Nodes[0].ID, nil
}

// ResolveIssueUUID returns the UUID for an issue identifier.
func ResolveIssueUUID(ctx context.Context, client *graphql.Client, identifier string) (string, error) {
	data, err := client.RequestRaw(ctx, `query($id: String!) { issue(id: $id) { id } }`, map[string]any{"id": identifier})
	if err != nil {
		return "", err
	}
	var parsed struct {
		Issue *struct {
			ID string `json:"id"`
		} `json:"issue"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", err
	}
	if parsed.Issue == nil {
		return "", errors.NewNotFoundError("Issue", identifier)
	}
	return parsed.Issue.ID, nil
}

// ResolveProjectID resolves a project UUID, slug, or name.
func ResolveProjectID(ctx context.Context, client *graphql.Client, ref string) (string, error) {
	if ids.IsUUID(ref) {
		return ref, nil
	}
	data, err := client.RequestRaw(ctx, `
query($name: String!) {
  projects(filter: { or: [
    { name: { eqIgnoreCase: $name } }
    { slugId: { eq: $name } }
  ]}, first: 5) { nodes { id } }
}`, map[string]any{"name": ref})
	if err != nil {
		return "", err
	}
	var parsed struct {
		Projects struct {
			Nodes []struct {
				ID string `json:"id"`
			} `json:"nodes"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Projects.Nodes) == 0 {
		return "", errors.NewNotFoundError("Project", ref)
	}
	return parsed.Projects.Nodes[0].ID, nil
}

// ResolveWorkflowStateID finds a team workflow state by name or type.
func ResolveWorkflowStateID(ctx context.Context, client *graphql.Client, teamID, ref string) (string, error) {
	data, err := client.RequestRaw(ctx, `
query($teamId: String!) {
  team(id: $teamId) {
    states { nodes { id name type } }
  }
}`, map[string]any{"teamId": teamID})
	if err != nil {
		return "", err
	}
	var parsed struct {
		Team *struct {
			States struct {
				Nodes []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
					Type string `json:"type"`
				} `json:"nodes"`
			} `json:"states"`
		} `json:"team"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil || parsed.Team == nil {
		return "", errors.NewNotFoundError("Workflow state", ref)
	}
	lower := strings.ToLower(ref)
	for _, s := range parsed.Team.States.Nodes {
		if strings.EqualFold(s.Name, ref) || strings.EqualFold(s.Type, ref) || strings.EqualFold(s.Type, lower) {
			return s.ID, nil
		}
	}
	return "", errors.NewNotFoundError("Workflow state", ref)
}

// ResolveMilestoneID resolves a milestone UUID or name within an optional project.
func ResolveMilestoneID(ctx context.Context, client *graphql.Client, ref, projectID string) (string, error) {
	if ids.IsUUID(ref) {
		return ref, nil
	}
	if projectID == "" {
		return "", errors.NewValidationError(
			"milestone name requires --project",
			errors.WithSuggestion("Pass --project or a milestone UUID."),
		)
	}
	data, err := client.RequestRaw(ctx, `
query($projectId: String!) {
  project(id: $projectId) {
    projectMilestones(first: 100) { nodes { id name } }
  }
}`, map[string]any{"projectId": projectID})
	if err != nil {
		return "", err
	}
	var parsed struct {
		Project *struct {
			ProjectMilestones struct {
				Nodes []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"nodes"`
			} `json:"projectMilestones"`
		} `json:"project"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil || parsed.Project == nil {
		return "", errors.NewNotFoundError("Milestone", ref)
	}
	for _, m := range parsed.Project.ProjectMilestones.Nodes {
		if strings.EqualFold(m.Name, ref) {
			return m.ID, nil
		}
	}
	return "", errors.NewNotFoundError("Milestone", ref)
}

// ResolveCycleID resolves cycle number, name, or keyword within a team.
func ResolveCycleID(ctx context.Context, client *graphql.Client, teamID, ref string) (string, error) {
	if ids.IsUUID(ref) {
		return ref, nil
	}
	data, err := client.RequestRaw(ctx, `
query($teamId: String!) {
  team(id: $teamId) {
    cycles(first: 100) {
      nodes { id number name isActive isNext isPrevious isFuture isPast }
    }
  }
}`, map[string]any{"teamId": teamID})
	if err != nil {
		return "", err
	}
	var parsed struct {
		Team *struct {
			Cycles struct {
				Nodes []struct {
					ID         string  `json:"id"`
					Number     int     `json:"number"`
					Name       *string `json:"name"`
					IsActive   bool    `json:"isActive"`
					IsNext     bool    `json:"isNext"`
					IsPrevious bool    `json:"isPrevious"`
				} `json:"nodes"`
			} `json:"cycles"`
		} `json:"team"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil || parsed.Team == nil {
		return "", errors.NewNotFoundError("Cycle", ref)
	}
	lower := strings.ToLower(ref)
	switch lower {
	case "active", "now":
		for _, c := range parsed.Team.Cycles.Nodes {
			if c.IsActive {
				return c.ID, nil
			}
		}
	case "next":
		for _, c := range parsed.Team.Cycles.Nodes {
			if c.IsNext {
				return c.ID, nil
			}
		}
	case "previous", "prev":
		for _, c := range parsed.Team.Cycles.Nodes {
			if c.IsPrevious {
				return c.ID, nil
			}
		}
	}
	var wantNum int
	if _, err := fmt.Sscanf(ref, "%d", &wantNum); err == nil {
		for _, c := range parsed.Team.Cycles.Nodes {
			if c.Number == wantNum {
				return c.ID, nil
			}
		}
	}
	for _, c := range parsed.Team.Cycles.Nodes {
		if c.Name != nil && strings.EqualFold(*c.Name, ref) {
			return c.ID, nil
		}
	}
	return "", errors.NewNotFoundError("Cycle", ref)
}

// ResolveLabelIDs resolves label names for a team (plus workspace labels).
func ResolveLabelIDs(ctx context.Context, client *graphql.Client, teamKey string, names []string) ([]string, error) {
	ids := make([]string, 0, len(names))
	for _, name := range names {
		data, err := client.RequestRaw(ctx, `
query($name: String!) {
  issueLabels(filter: { name: { eqIgnoreCase: $name } }) {
    nodes { id name team { key } }
  }
}`, map[string]any{"name": name})
		if err != nil {
			return nil, err
		}
		var parsed struct {
			IssueLabels struct {
				Nodes []struct {
					ID   string `json:"id"`
					Team *struct {
						Key string `json:"key"`
					} `json:"team"`
				} `json:"nodes"`
			} `json:"issueLabels"`
		}
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, err
		}
		var chosen string
		for _, n := range parsed.IssueLabels.Nodes {
			if n.Team != nil && strings.EqualFold(n.Team.Key, teamKey) {
				chosen = n.ID
				break
			}
		}
		if chosen == "" {
			for _, n := range parsed.IssueLabels.Nodes {
				if n.Team == nil {
					chosen = n.ID
					break
				}
			}
		}
		if chosen == "" && len(parsed.IssueLabels.Nodes) > 0 {
			chosen = parsed.IssueLabels.Nodes[0].ID
		}
		if chosen == "" {
			return nil, errors.NewNotFoundError("Label", name)
		}
		ids = append(ids, chosen)
	}
	return ids, nil
}

// ReadDescriptionFile reads a description from a file path.
func ReadDescriptionFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", errors.NewValidationError(fmt.Sprintf("Failed to read description file: %s", path))
	}
	return string(b), nil
}

func viewerID(ctx context.Context, client *graphql.Client) (string, error) {
	const q = `query { viewer { id } }`
	data, err := client.RequestRaw(ctx, q, nil)
	if err != nil {
		return "", err
	}
	var parsed struct {
		Viewer struct {
			ID string `json:"id"`
		} `json:"viewer"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", err
	}
	return parsed.Viewer.ID, nil
}

package issues

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/kongken/linear-cli/internal/errors"
	"github.com/kongken/linear-cli/internal/graphql"
	"github.com/kongken/linear-cli/internal/teams"
)

const issueBranchQuery = `
query IssueBranch($id: String!) {
  issue(id: $id) {
    id
    identifier
    title
    branchName
    team { id key }
  }
}
`

const issueUpdateMutation = `
mutation IssueUpdate($id: String!, $input: IssueUpdateInput!) {
  issueUpdate(id: $id, input: $input) {
    success
    issue { id identifier title url }
  }
}
`

const startedStateQuery = `
query StartedState($teamId: String!) {
  team(id: $teamId) {
    states {
      nodes { id name type position }
    }
  }
}
`

// StartWork checks out/creates the issue branch and moves the issue to started.
func StartWork(ctx context.Context, issueIdentifier, fromRef, customBranch string) error {
	client, err := graphql.NewClient()
	if err != nil {
		return err
	}
	data, err := client.RequestRaw(ctx, issueBranchQuery, map[string]any{"id": issueIdentifier})
	if err != nil {
		return err
	}
	var parsed struct {
		Issue *struct {
			ID         string `json:"id"`
			Identifier string `json:"identifier"`
			Title      string `json:"title"`
			BranchName string `json:"branchName"`
			Team       struct {
				ID  string `json:"id"`
				Key string `json:"key"`
			} `json:"team"`
		} `json:"issue"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	if parsed.Issue == nil {
		return errors.NewNotFoundError("Issue", issueIdentifier)
	}
	branch := customBranch
	if branch == "" {
		branch = parsed.Issue.BranchName
	}
	if branch == "" {
		branch = strings.ToLower(parsed.Issue.Identifier)
	}

	if err := checkoutBranch(branch, fromRef); err != nil {
		return err
	}

	stateID, err := startedStateID(ctx, client, parsed.Issue.Team.ID)
	if err != nil {
		return err
	}
	if stateID != "" {
		_, err = UpdateIssue(ctx, parsed.Issue.ID, map[string]any{"stateId": stateID})
		if err != nil {
			return err
		}
	}
	fmt.Printf("Started %s on branch %s\n", parsed.Issue.Identifier, branch)
	return nil
}

func checkoutBranch(branch, fromRef string) error {
	// Prefer creating from fromRef when provided.
	args := []string{"checkout", "-B", branch}
	if fromRef != "" {
		args = append(args, fromRef)
	}
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.NewCliError(
			fmt.Sprintf("Failed to checkout branch %q: %s", branch, strings.TrimSpace(string(out))),
			errors.WithCause(err),
		)
	}
	return nil
}

func startedStateID(ctx context.Context, client *graphql.Client, teamID string) (string, error) {
	data, err := client.RequestRaw(ctx, startedStateQuery, map[string]any{"teamId": teamID})
	if err != nil {
		return "", err
	}
	var parsed struct {
		Team *struct {
			States struct {
				Nodes []struct {
					ID       string  `json:"id"`
					Type     string  `json:"type"`
					Position float64 `json:"position"`
				} `json:"nodes"`
			} `json:"states"`
		} `json:"team"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", err
	}
	if parsed.Team == nil {
		return "", nil
	}
	var bestID string
	bestPos := -1.0
	for _, s := range parsed.Team.States.Nodes {
		if s.Type != "started" {
			continue
		}
		if bestID == "" || s.Position < bestPos {
			bestID = s.ID
			bestPos = s.Position
		}
	}
	return bestID, nil
}

// UpdateIssue applies IssueUpdateInput fields by Linear issue UUID or identifier.
func UpdateIssue(ctx context.Context, issueID string, input map[string]any) (json.RawMessage, error) {
	client, err := graphql.NewClient()
	if err != nil {
		return nil, err
	}
	data, err := client.RequestRaw(ctx, issueUpdateMutation, map[string]any{
		"id":    issueID,
		"input": input,
	})
	if err != nil {
		return nil, err
	}
	var parsed struct {
		IssueUpdate struct {
			Success bool            `json:"success"`
			Issue   json.RawMessage `json:"issue"`
		} `json:"issueUpdate"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	if !parsed.IssueUpdate.Success {
		return nil, errors.NewCliError("Issue update failed")
	}
	return parsed.IssueUpdate.Issue, nil
}

// ResolveTeamID resolves a team key to UUID (helper for callers).
func ResolveTeamID(ctx context.Context, key string) (string, error) {
	t, err := teams.Resolve(ctx, key)
	if err != nil {
		return "", err
	}
	return t.ID, nil
}

package linear

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"

	"github.com/kongken/linear-cli/internal/config"
	"github.com/kongken/linear-cli/internal/display"
	"github.com/kongken/linear-cli/internal/errors"
	"github.com/kongken/linear-cli/internal/gql"
	"github.com/kongken/linear-cli/internal/graphql"
	"github.com/kongken/linear-cli/internal/issueid"
	"github.com/kongken/linear-cli/internal/vcs"
)

var integerIDRe = regexp.MustCompile(`^[1-9][0-9]*$`)

// Matches Linear issue details field selection for --json output.
const issueDetailsQuery = `
query GetIssueDetails($id: String!) {
  issue(id: $id) {
    identifier
    title
    description
    url
    branchName
    state { name color }
    assignee { name displayName }
    priority
    project { name }
    projectMilestone { name }
    cycle {
      id number name isActive isNext isPrevious isFuture isPast
    }
    team { activeCycle { number } }
    labels(first: 50) { nodes { id name color } }
    parent { identifier title state { name color } }
    children(first: 250) { nodes { identifier title state { name color } } }
    attachments(first: 50) {
      nodes { id title url subtitle sourceType metadata createdAt }
    }
    documents(first: 50) {
      nodes { id title slugId url createdAt updatedAt }
    }
  }
}
`

// IssueTitleURL is a minimal issue payload for title/url commands.
type IssueTitleURL struct {
	Title      string `json:"title"`
	URL        string `json:"url"`
	Identifier string `json:"identifier"`
}

// IssueIdentifier resolves a Linear issue id from an optional provided value or VCS state.
func IssueIdentifier(providedID string) (string, error) {
	if providedID != "" {
		if normalized, ok := issueid.NormalizeIssueIdentifier(providedID); ok {
			return normalized, nil
		}
		if integerIDRe.MatchString(providedID) {
			team, ok := config.GetOption("team_id")
			if !ok || team == "" {
				return "", errors.NewValidationError(
					"an integer id was provided, but no team is set",
					errors.WithSuggestion("Run `linear config` to set a team."),
				)
			}
			if normalized, ok := issueid.NormalizeIssueIdentifier(team + "-" + providedID); ok {
				return normalized, nil
			}
		}
		return "", nil
	}

	return vcs.CurrentIssueIdentifier()
}

// FetchIssueTitleURL loads title and url for an issue identifier.
func FetchIssueTitleURL(ctx context.Context, issueID string) (*IssueTitleURL, error) {
	client, err := graphql.NewClient()
	if err != nil {
		return nil, err
	}
	resp, err := gql.GetIssueTitleURL(ctx, client, issueID)
	if err != nil {
		return nil, err
	}
	if resp.Issue.Identifier == "" && resp.Issue.Title == "" && resp.Issue.Url == "" {
		return nil, errors.NewNotFoundError("Issue", issueID)
	}
	return &IssueTitleURL{
		Title:      resp.Issue.Title,
		URL:        resp.Issue.Url,
		Identifier: resp.Issue.Identifier,
	}, nil
}

// FetchIssueDetailsJSON returns the raw GraphQL issue object (for --json).
func FetchIssueDetailsJSON(ctx context.Context, issueID string) (json.RawMessage, error) {
	client, err := graphql.NewClient()
	if err != nil {
		return nil, err
	}
	data, err := client.RequestRaw(ctx, issueDetailsQuery, map[string]any{"id": issueID})
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Issue json.RawMessage `json:"issue"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}
	if len(wrapper.Issue) == 0 || string(wrapper.Issue) == "null" {
		return nil, errors.NewNotFoundError("Issue", issueID)
	}
	return wrapper.Issue, nil
}

// FormatIssueDetailsText renders a simple human-readable issue summary.
func FormatIssueDetailsText(raw json.RawMessage) (string, error) {
	var issue struct {
		Identifier  string  `json:"identifier"`
		Title       string  `json:"title"`
		Description *string `json:"description"`
		URL         string  `json:"url"`
		State       *struct {
			Name string `json:"name"`
		} `json:"state"`
		Assignee *struct {
			Name string `json:"name"`
		} `json:"assignee"`
		Priority *float64 `json:"priority"`
		Project  *struct {
			Name string `json:"name"`
		} `json:"project"`
	}
	if err := json.Unmarshal(raw, &issue); err != nil {
		return "", err
	}
	out := fmt.Sprintf("# %s %s\n", issue.Identifier, issue.Title)
	if issue.State != nil {
		out += fmt.Sprintf("State: %s\n", issue.State.Name)
	}
	if issue.Assignee != nil {
		out += fmt.Sprintf("Assignee: %s\n", issue.Assignee.Name)
	}
	if issue.Project != nil {
		out += fmt.Sprintf("Project: %s\n", issue.Project.Name)
	}
	if issue.URL != "" {
		out += fmt.Sprintf("URL: %s\n", issue.URL)
	}
	if issue.Description != nil && *issue.Description != "" {
		desc := *issue.Description
		if os.Getenv("LINEAR_DOWNLOAD_IMAGES") != "0" {
			headers := map[string]string{}
			if key, err := graphql.ResolvedAPIKey(); err == nil && key != "" {
				headers["Authorization"] = key
			}
			prepared, err := display.PrepareMarkdown(desc, true, headers)
			if err == nil {
				desc = prepared
			}
		}
		out += "\n" + desc + "\n"
	}
	rendered, err := display.Markdown(out)
	if err != nil {
		return out, nil
	}
	return rendered, nil
}

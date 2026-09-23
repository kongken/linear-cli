package issues

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kongken/linear-cli/internal/errors"
	"github.com/kongken/linear-cli/internal/graphql"
)

const issuesForStateQuery = `
query GetIssuesForState($sort: [IssueSortInput!], $filter: IssueFilter!, $first: Int, $after: String) {
  issues(filter: $filter, sort: $sort, first: $first, after: $after) {
    nodes {
      id
      identifier
      title
      priority
      state {
        id
        name
        color
        type
      }
      assignee {
        initials
        name
      }
      team {
        key
      }
    }
    pageInfo {
      hasNextPage
      endCursor
    }
  }
}
`

// MineOptions configures issue mine / state listing.
type MineOptions struct {
	TeamKey      string
	StateType    string // empty = no state filter; e.g. "unstarted"
	Limit        int    // 0 = unlimited (capped page fetch)
	Sort         string // "priority" or "manual"
	Unassigned   bool   // only issues with no assignee
	AllAssignees bool   // no assignee filter
	Project      string
	Labels       []string
	CreatedAfter string
}

// IssueSummary is a compact issue row for selection UIs.
type IssueSummary struct {
	Identifier string  `json:"identifier"`
	Title      string  `json:"title"`
	Priority   float64 `json:"priority"`
}

// FetchMineIssues returns issues for a team (defaults to assignee=self).
func FetchMineIssues(ctx context.Context, opts MineOptions) (json.RawMessage, error) {
	if opts.TeamKey == "" {
		return nil, errors.NewValidationError(
			"No default team configured and no team scope provided",
			errors.WithSuggestion("Use --team <key> to specify a team, or run `linear config` to link this repository to a team."),
		)
	}
	if opts.Unassigned && opts.AllAssignees {
		return nil, errors.NewValidationError("Cannot specify both unassigned and all-assignees filters")
	}

	client, err := graphql.NewClient()
	if err != nil {
		return nil, err
	}

	filter := map[string]any{
		"team": map[string]any{"key": map[string]any{"eq": opts.TeamKey}},
	}
	switch {
	case opts.Unassigned:
		filter["assignee"] = map[string]any{"null": true}
	case opts.AllAssignees:
		// no assignee filter
	default:
		filter["assignee"] = map[string]any{"isMe": map[string]any{"eq": true}}
	}
	if opts.StateType != "" {
		filter["state"] = map[string]any{"type": map[string]any{"eq": opts.StateType}}
	}
	if opts.Project != "" {
		pid, err := ResolveProjectID(ctx, client, opts.Project)
		if err != nil {
			return nil, err
		}
		filter["project"] = map[string]any{"id": map[string]any{"eq": pid}}
	}
	if len(opts.Labels) == 1 {
		filter["labels"] = map[string]any{
			"some": map[string]any{"name": map[string]any{"eqIgnoreCase": opts.Labels[0]}},
		}
	} else if len(opts.Labels) > 1 {
		ands := make([]any, 0, len(opts.Labels))
		for _, name := range opts.Labels {
			ands = append(ands, map[string]any{
				"some": map[string]any{"name": map[string]any{"eqIgnoreCase": name}},
			})
		}
		filter["labels"] = map[string]any{"and": ands}
	}
	if opts.CreatedAfter != "" {
		gte, err := parseDateFilter(opts.CreatedAfter, "--created-after")
		if err != nil {
			return nil, err
		}
		filter["createdAt"] = map[string]any{"gte": gte}
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	first := limit
	if first > 50 {
		first = 50
	}

	var sort any
	switch opts.Sort {
	case "manual":
		sort = []map[string]any{{"manual": map[string]any{"nulls": "last", "order": "Ascending"}}}
	default:
		sort = []map[string]any{{"priority": map[string]any{"nulls": "last", "order": "Ascending"}}}
	}

	var allNodes []json.RawMessage
	var pageInfo json.RawMessage
	var after *string

	for {
		vars := map[string]any{
			"filter": filter,
			"sort":   sort,
			"first":  first,
		}
		if after != nil {
			vars["after"] = *after
		}
		data, err := client.RequestRaw(ctx, issuesForStateQuery, vars)
		if err != nil {
			return nil, err
		}
		var parsed struct {
			Issues struct {
				Nodes    []json.RawMessage `json:"nodes"`
				PageInfo struct {
					HasNextPage bool    `json:"hasNextPage"`
					EndCursor   *string `json:"endCursor"`
				} `json:"pageInfo"`
			} `json:"issues"`
		}
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, err
		}
		allNodes = append(allNodes, parsed.Issues.Nodes...)
		pi, _ := json.Marshal(parsed.Issues.PageInfo)
		pageInfo = pi
		if !parsed.Issues.PageInfo.HasNextPage || len(allNodes) >= limit || parsed.Issues.PageInfo.EndCursor == nil {
			break
		}
		after = parsed.Issues.PageInfo.EndCursor
	}
	if len(allNodes) > limit {
		allNodes = allNodes[:limit]
	}

	out, err := json.Marshal(map[string]any{
		"nodes":    allNodes,
		"pageInfo": json.RawMessage(pageInfo),
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ParseIssueSummaries extracts identifier/title/priority from a connection payload.
func ParseIssueSummaries(payload json.RawMessage) ([]IssueSummary, error) {
	var parsed struct {
		Nodes []IssueSummary `json:"nodes"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return nil, err
	}
	return parsed.Nodes, nil
}

// FormatMineText prints a compact identifier/title/state listing.
func FormatMineText(payload json.RawMessage) (string, error) {
	var parsed struct {
		Nodes []struct {
			Identifier string `json:"identifier"`
			Title      string `json:"title"`
			State      *struct {
				Name string `json:"name"`
			} `json:"state"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Nodes) == 0 {
		return "No issues found\n", nil
	}
	out := ""
	for _, n := range parsed.Nodes {
		state := ""
		if n.State != nil {
			state = n.State.Name
		}
		out += fmt.Sprintf("%s\t%s\t%s\n", n.Identifier, state, n.Title)
	}
	return out, nil
}

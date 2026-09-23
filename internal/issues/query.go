package issues

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kongken/linear-cli/internal/errors"
	"github.com/kongken/linear-cli/internal/graphql"
)

const queryIssuesQuery = `
query GetIssuesForQuery(
  $sort: [IssueSortInput!]
  $filter: IssueFilter
  $first: Int
  $after: String
  $includeArchived: Boolean
) {
  issues(
    filter: $filter
    sort: $sort
    first: $first
    after: $after
    includeArchived: $includeArchived
  ) {
    nodes {
      id
      identifier
      title
      url
      priority
      priorityLabel
      estimate
      createdAt
      updatedAt
      state { id name color type position }
      assignee { id name displayName initials }
      team { id key name }
      project { id name }
      labels { nodes { id name color } }
    }
    pageInfo { hasNextPage endCursor }
  }
}
`

const searchIssuesQuery = `
query SearchIssues($term: String!, $filter: IssueFilter, $first: Int, $after: String) {
  searchIssues(term: $term, filter: $filter, first: $first, after: $after) {
    nodes {
      id
      identifier
      title
      url
      priority
      state { id name color type }
      assignee { id name displayName }
      team { id key name }
    }
    pageInfo { hasNextPage endCursor }
  }
}
`

// QueryOptions configures issue query.
type QueryOptions struct {
	TeamKeys        []string
	AllTeams        bool
	StateType       string // optional workflow state type
	Assignee        string // username / self / @me
	AssigneeMe      bool   // --mine shorthand
	Unassigned      bool
	Project         string
	ProjectLabel    string
	Labels          []string
	CreatedAfter    string
	UpdatedAfter    string
	IncludeArchived bool
	Search          string
	Limit           int
	Sort            string
}

// FetchQueryIssues returns a connection payload {nodes,pageInfo}.
func FetchQueryIssues(ctx context.Context, opts QueryOptions) (json.RawMessage, error) {
	if !opts.AllTeams && len(opts.TeamKeys) == 0 {
		return nil, errors.NewValidationError(
			"No team scope provided",
			errors.WithSuggestion("Use --team <key>, --all-teams, or configure team_id via `linear config`."),
		)
	}
	if opts.Project != "" && opts.ProjectLabel != "" {
		return nil, errors.NewValidationError(
			"Cannot use --project and --project-label together",
			errors.WithSuggestion("Use --project for a single project, or --project-label for projects with a given label."),
		)
	}
	assigneeFilters := 0
	if opts.Assignee != "" {
		assigneeFilters++
	}
	if opts.AssigneeMe {
		assigneeFilters++
	}
	if opts.Unassigned {
		assigneeFilters++
	}
	if assigneeFilters > 1 {
		return nil, errors.NewValidationError(
			"Cannot specify multiple assignee filters (--assignee, --mine, --unassigned)",
		)
	}

	client, err := graphql.NewClient()
	if err != nil {
		return nil, err
	}

	filter := map[string]any{}
	if !opts.AllTeams {
		if len(opts.TeamKeys) == 1 {
			filter["team"] = map[string]any{"key": map[string]any{"eq": opts.TeamKeys[0]}}
		} else {
			ors := make([]any, 0, len(opts.TeamKeys))
			for _, k := range opts.TeamKeys {
				ors = append(ors, map[string]any{"key": map[string]any{"eq": k}})
			}
			filter["team"] = map[string]any{"or": ors}
		}
	}
	if opts.StateType != "" {
		filter["state"] = map[string]any{"type": map[string]any{"eq": opts.StateType}}
	}
	if opts.Unassigned {
		filter["assignee"] = map[string]any{"null": true}
	} else if opts.AssigneeMe {
		filter["assignee"] = map[string]any{"isMe": map[string]any{"eq": true}}
	} else if opts.Assignee != "" {
		uid, err := ResolveUserID(ctx, client, opts.Assignee)
		if err != nil {
			return nil, err
		}
		filter["assignee"] = map[string]any{"id": map[string]any{"eq": uid}}
	}

	if opts.Project != "" {
		pid, err := ResolveProjectID(ctx, client, opts.Project)
		if err != nil {
			return nil, err
		}
		filter["project"] = map[string]any{"id": map[string]any{"eq": pid}}
	} else if opts.ProjectLabel != "" {
		filter["project"] = map[string]any{
			"labels": map[string]any{"name": map[string]any{"eqIgnoreCase": opts.ProjectLabel}},
		}
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
	if opts.UpdatedAfter != "" {
		gte, err := parseDateFilter(opts.UpdatedAfter, "--updated-after")
		if err != nil {
			return nil, err
		}
		filter["updatedAt"] = map[string]any{"gte": gte}
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}

	if opts.Search != "" {
		return paginate(ctx, client, searchIssuesQuery, "searchIssues", filter, opts.Search, limit, nil, opts.IncludeArchived)
	}

	var sort any
	switch opts.Sort {
	case "manual":
		sort = []map[string]any{{"manual": map[string]any{"nulls": "last", "order": "Ascending"}}}
	default:
		sort = []map[string]any{{"priority": map[string]any{"nulls": "last", "order": "Ascending"}}}
	}
	return paginate(ctx, client, queryIssuesQuery, "issues", filter, "", limit, sort, opts.IncludeArchived)
}

func parseDateFilter(raw, flag string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.NewValidationError(flag + " requires a date")
	}
	// Accept YYYY-MM-DD or full RFC3339/ISO timestamps.
	if len(raw) == 10 && raw[4] == '-' && raw[7] == '-' {
		return raw + "T00:00:00.000Z", nil
	}
	if _, err := time.Parse(time.RFC3339, raw); err == nil {
		return raw, nil
	}
	if _, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return raw, nil
	}
	return "", errors.NewValidationError(
		fmt.Sprintf("Invalid date for %s: %s", flag, raw),
		errors.WithSuggestion("Use YYYY-MM-DD or an ISO-8601 timestamp."),
	)
}

func paginate(
	ctx context.Context,
	client *graphql.Client,
	query string,
	root string,
	filter map[string]any,
	search string,
	limit int,
	sort any,
	includeArchived bool,
) (json.RawMessage, error) {
	var allNodes []json.RawMessage
	var pageInfo json.RawMessage
	var after *string
	first := limit
	if first > 50 {
		first = 50
	}

	for {
		vars := map[string]any{"first": first, "includeArchived": includeArchived}
		if len(filter) > 0 {
			vars["filter"] = filter
		}
		if sort != nil {
			vars["sort"] = sort
		}
		if search != "" {
			vars["term"] = search
		}
		if after != nil {
			vars["after"] = *after
		}
		data, err := client.RequestRaw(ctx, query, vars)
		if err != nil {
			return nil, err
		}
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(data, &envelope); err != nil {
			return nil, err
		}
		connRaw, ok := envelope[root]
		if !ok {
			return nil, fmt.Errorf("missing %s in response", root)
		}
		var conn struct {
			Nodes    []json.RawMessage `json:"nodes"`
			PageInfo struct {
				HasNextPage bool    `json:"hasNextPage"`
				EndCursor   *string `json:"endCursor"`
			} `json:"pageInfo"`
		}
		if err := json.Unmarshal(connRaw, &conn); err != nil {
			return nil, err
		}
		allNodes = append(allNodes, conn.Nodes...)
		pi, _ := json.Marshal(conn.PageInfo)
		pageInfo = pi
		if !conn.PageInfo.HasNextPage || len(allNodes) >= limit || conn.PageInfo.EndCursor == nil {
			break
		}
		after = conn.PageInfo.EndCursor
	}
	if len(allNodes) > limit {
		allNodes = allNodes[:limit]
	}
	return json.Marshal(map[string]any{
		"nodes":    allNodes,
		"pageInfo": json.RawMessage(pageInfo),
	})
}

// FormatQueryText prints identifier/state/title lines.
func FormatQueryText(payload json.RawMessage) (string, error) {
	return FormatMineText(payload)
}

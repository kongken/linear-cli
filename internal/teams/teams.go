package teams

import (
	"context"
	"strings"

	"github.com/kongken/linear-cli/internal/errors"
	"github.com/kongken/linear-cli/internal/gql"
	"github.com/kongken/linear-cli/internal/graphql"
)

// Team is a resolved Linear team.
type Team struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

// Resolve looks up a team by key or name (case-insensitive).
func Resolve(ctx context.Context, reference string) (*Team, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return nil, errors.NewValidationError(
			"Team reference is empty",
			errors.WithSuggestion("Pass a team key, name, or ID, e.g. --team ENG."),
		)
	}
	client, err := graphql.NewClient()
	if err != nil {
		return nil, err
	}
	data, err := gql.ResolveTeam(ctx, client, reference)
	if err != nil {
		return nil, err
	}
	if len(data.Teams.Nodes) == 0 {
		return nil, errors.NewNotFoundError("Team", reference)
	}
	n := data.Teams.Nodes[0]
	return &Team{ID: n.Id, Key: n.Key, Name: n.Name}, nil
}

// ListAll returns all teams the viewer can access.
func ListAll(ctx context.Context) ([]Team, error) {
	client, err := graphql.NewClient()
	if err != nil {
		return nil, err
	}
	var all []Team
	var after *string
	first := 50
	for {
		data, err := gql.ListTeams(ctx, client, &first, after)
		if err != nil {
			return nil, err
		}
		for _, n := range data.Teams.Nodes {
			all = append(all, Team{ID: n.Id, Key: n.Key, Name: n.Name})
		}
		if !data.Teams.PageInfo.HasNextPage || data.Teams.PageInfo.EndCursor == nil || *data.Teams.PageInfo.EndCursor == "" {
			break
		}
		after = data.Teams.PageInfo.EndCursor
	}
	return all, nil
}

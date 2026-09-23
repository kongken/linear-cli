package cmdproject

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/kongken/linear-cli/internal/config"
	"github.com/kongken/linear-cli/internal/errors"
	"github.com/kongken/linear-cli/internal/graphql"
	"github.com/kongken/linear-cli/internal/prompt"
	"github.com/kongken/linear-cli/internal/teams"
)

type interactiveProjectCreate struct {
	Name        string
	Description string
	TeamKey     string
	Status      string
	Lead        string
}

func runInteractiveProjectCreate(seedTeam string) (*interactiveProjectCreate, error) {
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, "Create a new project")
	fmt.Fprintln(os.Stdout, "")

	name, err := prompt.Text("Project name:", "")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(name) == "" {
		return nil, errors.NewValidationError("Project name is required")
	}

	description, err := prompt.Text("Description (optional):", "")
	if err != nil {
		return nil, err
	}

	teamKey := strings.ToUpper(strings.TrimSpace(seedTeam))
	if teamKey == "" {
		if t, ok := config.GetOption("team_id"); ok {
			teamKey = strings.ToUpper(t)
		}
	}
	if teamKey == "" {
		all, err := teams.ListAll(context.Background())
		if err != nil {
			return nil, err
		}
		if len(all) == 0 {
			return nil, errors.NewNotFoundError("Team", "(none)")
		}
		opts := make([]prompt.Option, 0, len(all))
		for _, t := range all {
			opts = append(opts, prompt.Option{
				Label: fmt.Sprintf("%s (%s)", t.Name, t.Key),
				Value: t.Key,
			})
		}
		selected, err := prompt.Select("Team:", opts)
		if err != nil {
			return nil, err
		}
		teamKey = selected
	}

	status := ""
	if client, err := graphql.NewClient(); err == nil {
		data, err := client.RequestRaw(context.Background(), `
query GetProjectStatuses {
  projectStatuses {
    nodes { id name type }
  }
}`, nil)
		if err == nil {
			var parsed struct {
				ProjectStatuses struct {
					Nodes []struct {
						Name string `json:"name"`
						Type string `json:"type"`
					} `json:"nodes"`
				} `json:"projectStatuses"`
			}
			if json.Unmarshal(data, &parsed) == nil && len(parsed.ProjectStatuses.Nodes) > 0 {
				opts := make([]prompt.Option, 0, len(parsed.ProjectStatuses.Nodes))
				for _, s := range parsed.ProjectStatuses.Nodes {
					opts = append(opts, prompt.Option{Label: s.Name, Value: s.Type})
				}
				selected, err := prompt.Select("Status:", opts)
				if err != nil {
					return nil, err
				}
				status = selected
			}
		}
	}

	lead, err := prompt.Text("Lead (optional, @me/username/email):", "")
	if err != nil {
		return nil, err
	}

	return &interactiveProjectCreate{
		Name:        strings.TrimSpace(name),
		Description: strings.TrimSpace(description),
		TeamKey:     teamKey,
		Status:      status,
		Lead:        strings.TrimSpace(lead),
	}, nil
}

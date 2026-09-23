package cmdlabel

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/kongken/linear-cli/internal/config"
	"github.com/kongken/linear-cli/internal/errors"
	"github.com/kongken/linear-cli/internal/gql"
	"github.com/kongken/linear-cli/internal/graphql"
	"github.com/kongken/linear-cli/internal/prompt"
	"github.com/kongken/linear-cli/internal/teams"
	"github.com/spf13/cobra"
)

const getIssueLabelsQuery = `
query GetIssueLabels($filter: IssueLabelFilter, $first: Int, $after: String) {
  issueLabels(filter: $filter, first: $first, after: $after) {
    nodes {
      id
      name
      description
      color
      team { key name }
    }
    pageInfo { hasNextPage endCursor }
  }
}
`

var uuidRE = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// New returns the label command group.
func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "label",
		Short: "Manage Linear labels",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newListCommand(), newCreateCommand(), newDeleteCommand())
	return cmd
}

func newListCommand() *cobra.Command {
	var (
		team    string
		jsonOut bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List issue labels",
		Run: func(cmd *cobra.Command, args []string) {
			teamKey := strings.ToUpper(team)
			if teamKey == "" {
				if t, ok := config.GetOption("team_id"); ok {
					teamKey = strings.ToUpper(t)
				}
			}
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to list labels")
				os.Exit(1)
			}
			vars := map[string]any{"first": 100}
			if teamKey != "" {
				vars["filter"] = map[string]any{
					"team": map[string]any{"key": map[string]any{"eq": teamKey}},
				}
			}
			data, err := client.RequestRaw(context.Background(), getIssueLabelsQuery, vars)
			if err != nil {
				errors.HandleError(err, "Failed to list labels")
				os.Exit(1)
			}
			var parsed struct {
				IssueLabels struct {
					Nodes    []json.RawMessage `json:"nodes"`
					PageInfo json.RawMessage   `json:"pageInfo"`
				} `json:"issueLabels"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil {
				errors.HandleError(err, "Failed to list labels")
				os.Exit(1)
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(map[string]any{
					"nodes":    parsed.IssueLabels.Nodes,
					"pageInfo": parsed.IssueLabels.PageInfo,
				})
				return
			}
			for _, raw := range parsed.IssueLabels.Nodes {
				var l struct {
					Name  string `json:"name"`
					Color string `json:"color"`
					Team  *struct {
						Key string `json:"key"`
					} `json:"team"`
				}
				_ = json.Unmarshal(raw, &l)
				scope := "Workspace"
				if l.Team != nil {
					scope = l.Team.Key
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", l.Name, l.Color, scope)
			}
		},
	}
	cmd.Flags().StringVarP(&team, "team", "t", "", "Filter by team key")
	cmd.Flags().BoolVarP(&jsonOut, "json", "j", false, "Output as JSON")
	return cmd
}

func newCreateCommand() *cobra.Command {
	var name, color, description, team string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an issue label",
		Run: func(cmd *cobra.Command, args []string) {
			if name == "" {
				errors.HandleError(
					errors.NewValidationError("Label name is required", errors.WithSuggestion("Use --name or -n")),
					"Failed to create label",
				)
				os.Exit(1)
			}
			if color == "" {
				color = "#5E6AD2"
			}
			if !regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`).MatchString(color) {
				errors.HandleError(
					errors.NewValidationError("Color must be a valid hex code (e.g., #EB5757)"),
					"Failed to create label",
				)
				os.Exit(1)
			}
			input := gql.IssueLabelCreateInput{Name: name, Color: &color}
			if description != "" {
				input.Description = &description
			}
			if team != "" {
				t, err := teams.Resolve(context.Background(), team)
				if err != nil {
					errors.HandleError(err, "Failed to create label")
					os.Exit(1)
				}
				input.TeamId = &t.ID
			}
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to create label")
				os.Exit(1)
			}
			resp, err := gql.CreateIssueLabel(context.Background(), client, input)
			if err != nil {
				errors.HandleError(err, "Failed to create label")
				os.Exit(1)
			}
			if !resp.IssueLabelCreate.Success {
				errors.HandleError(errors.NewCliError("Failed to create label"), "Failed to create label")
				os.Exit(1)
			}
			l := resp.IssueLabelCreate.IssueLabel
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Created label: %s\n", l.Name)
			fmt.Fprintf(cmd.OutOrStdout(), "  Color: %s\n", l.Color)
			if l.Description != nil && *l.Description != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  Description: %s\n", *l.Description)
			}
			scope := "Workspace"
			if l.Team != nil {
				scope = fmt.Sprintf("%s (%s)", l.Team.Name, l.Team.Key)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  Scope: %s\n", scope)
		},
	}
	cmd.Flags().StringVarP(&name, "name", "n", "", "Label name")
	cmd.Flags().StringVarP(&color, "color", "c", "", "Color hex code")
	cmd.Flags().StringVarP(&description, "description", "d", "", "Label description")
	cmd.Flags().StringVarP(&team, "team", "t", "", "Team for team-specific label")
	return cmd
}

func newDeleteCommand() *cobra.Command {
	var team string
	var force bool
	cmd := &cobra.Command{
		Use:   "delete <nameOrId>",
		Short: "Delete an issue label",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to delete label")
				os.Exit(1)
			}
			effectiveTeam := team
			if effectiveTeam == "" {
				if t, ok := config.GetOption("team_id"); ok {
					effectiveTeam = t
				}
			} else {
				resolved, err := teams.Resolve(context.Background(), team)
				if err != nil {
					errors.HandleError(err, "Failed to delete label")
					os.Exit(1)
				}
				effectiveTeam = resolved.Key
			}
			label, err := resolveLabel(client, args[0], effectiveTeam)
			if err != nil {
				errors.HandleError(err, "Failed to delete label")
				os.Exit(1)
			}
			scope := "Workspace"
			if label.TeamKey != "" {
				scope = label.TeamKey
			}
			labelDisplay := fmt.Sprintf("%s (%s)", label.Name, scope)
			ok, err := prompt.Confirm(fmt.Sprintf(`Are you sure you want to delete label "%s"?`, labelDisplay), force)
			if err != nil {
				errors.HandleError(err, "Failed to delete label")
				os.Exit(1)
			}
			if !ok {
				fmt.Fprintln(cmd.OutOrStdout(), "Deletion canceled")
				return
			}
			data, err := client.RequestRaw(context.Background(), `
mutation DeleteIssueLabel($id: String!) {
  issueLabelDelete(id: $id) { success }
}`, map[string]any{"id": label.ID})
			if err != nil {
				errors.HandleError(err, "Failed to delete label")
				os.Exit(1)
			}
			var parsed struct {
				IssueLabelDelete struct {
					Success bool `json:"success"`
				} `json:"issueLabelDelete"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil || !parsed.IssueLabelDelete.Success {
				errors.HandleError(errors.NewCliError("Failed to delete label"), "Failed to delete label")
				os.Exit(1)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Deleted label: %s\n", labelDisplay)
		},
	}
	cmd.Flags().StringVarP(&team, "team", "t", "", "Team key to disambiguate")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")
	return cmd
}

type labelInfo struct {
	ID      string
	Name    string
	TeamKey string
}

func resolveLabel(client *graphql.Client, nameOrID, teamKey string) (*labelInfo, error) {
	if uuidRE.MatchString(nameOrID) {
		data, err := client.RequestRaw(context.Background(), `
query GetLabelById($id: String!) {
  issueLabel(id: $id) { id name team { key } }
}`, map[string]any{"id": nameOrID})
		if err != nil {
			return nil, err
		}
		var parsed struct {
			IssueLabel *struct {
				ID   string `json:"id"`
				Name string `json:"name"`
				Team *struct {
					Key string `json:"key"`
				} `json:"team"`
			} `json:"issueLabel"`
		}
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, err
		}
		if parsed.IssueLabel == nil {
			return nil, errors.NewNotFoundError("Label", nameOrID)
		}
		info := &labelInfo{ID: parsed.IssueLabel.ID, Name: parsed.IssueLabel.Name}
		if parsed.IssueLabel.Team != nil {
			info.TeamKey = parsed.IssueLabel.Team.Key
		}
		return info, nil
	}
	data, err := client.RequestRaw(context.Background(), `
query GetLabelByName($name: String!) {
  issueLabels(filter: { name: { eqIgnoreCase: $name } }) {
    nodes { id name team { key } }
  }
}`, map[string]any{"name": nameOrID})
	if err != nil {
		return nil, err
	}
	var parsed struct {
		IssueLabels struct {
			Nodes []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
				Team *struct {
					Key string `json:"key"`
				} `json:"team"`
			} `json:"nodes"`
		} `json:"issueLabels"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	nodes := parsed.IssueLabels.Nodes
	if len(nodes) == 0 {
		return nil, errors.NewNotFoundError("Label", nameOrID)
	}
	if teamKey != "" {
		for _, n := range nodes {
			if n.Team != nil && strings.EqualFold(n.Team.Key, teamKey) {
				return &labelInfo{ID: n.ID, Name: n.Name, TeamKey: n.Team.Key}, nil
			}
		}
		for _, n := range nodes {
			if n.Team == nil {
				return &labelInfo{ID: n.ID, Name: n.Name}, nil
			}
		}
		return nil, errors.NewNotFoundError("Label", nameOrID)
	}
	if len(nodes) > 1 {
		return nil, errors.NewValidationError(
			fmt.Sprintf(`Multiple labels named "%s" found`, nameOrID),
			errors.WithSuggestion("Use --team to disambiguate."),
		)
	}
	info := &labelInfo{ID: nodes[0].ID, Name: nodes[0].Name}
	if nodes[0].Team != nil {
		info.TeamKey = nodes[0].Team.Key
	}
	return info, nil
}

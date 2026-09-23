package cmdteam

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/kongken/linear-cli/internal/config"
	"github.com/kongken/linear-cli/internal/errors"
	"github.com/kongken/linear-cli/internal/gql"
	"github.com/kongken/linear-cli/internal/graphql"
	"github.com/kongken/linear-cli/internal/prompt"
	"github.com/kongken/linear-cli/internal/teams"
	"github.com/spf13/cobra"
)

const getTeamsQuery = `
query GetTeams($filter: TeamFilter, $first: Int, $after: String) {
  teams(filter: $filter, first: $first, after: $after) {
    nodes {
      id name key description icon color cyclesEnabled
      createdAt updatedAt archivedAt
      organization { id name }
    }
    pageInfo { hasNextPage endCursor }
  }
}
`

const teamMembersQuery = `
query TeamMembers($teamId: String!, $includeDisabled: Boolean, $first: Int, $after: String) {
  team(id: $teamId) {
    members(includeDisabled: $includeDisabled, first: $first, after: $after) {
      nodes {
        id name displayName email active url admin guest
      }
      pageInfo { hasNextPage endCursor }
    }
  }
}
`

const teamStatesQuery = `
query TeamStates($teamId: String!) {
  team(id: $teamId) {
    states {
      nodes { id name type color position }
    }
  }
}
`

// New returns the team command group.
func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "team",
		Short: "Manage Linear teams",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		newListCommand(),
		newMembersCommand(),
		newStatesCommand(),
		newIDCommand(),
		newCreateCommand(),
		newDeleteCommand(),
		newAutolinksCommand(),
	)
	return cmd
}

func newListCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List teams",
		Run: func(cmd *cobra.Command, args []string) {
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to list teams")
				os.Exit(1)
			}
			var allNodes []json.RawMessage
			var pageInfo json.RawMessage
			var after *string
			for {
				vars := map[string]any{"first": 50}
				if after != nil {
					vars["after"] = *after
				}
				data, err := client.RequestRaw(context.Background(), getTeamsQuery, vars)
				if err != nil {
					errors.HandleError(err, "Failed to list teams")
					os.Exit(1)
				}
				var parsed struct {
					Teams struct {
						Nodes    []json.RawMessage `json:"nodes"`
						PageInfo struct {
							HasNextPage bool    `json:"hasNextPage"`
							EndCursor   *string `json:"endCursor"`
						} `json:"pageInfo"`
					} `json:"teams"`
				}
				if err := json.Unmarshal(data, &parsed); err != nil {
					errors.HandleError(err, "Failed to list teams")
					os.Exit(1)
				}
				allNodes = append(allNodes, parsed.Teams.Nodes...)
				pi, _ := json.Marshal(parsed.Teams.PageInfo)
				pageInfo = pi
				if !parsed.Teams.PageInfo.HasNextPage || parsed.Teams.PageInfo.EndCursor == nil {
					break
				}
				after = parsed.Teams.PageInfo.EndCursor
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(map[string]any{"nodes": allNodes, "pageInfo": json.RawMessage(pageInfo)})
				return
			}
			for _, raw := range allNodes {
				var team struct {
					Key  string `json:"key"`
					Name string `json:"name"`
				}
				_ = json.Unmarshal(raw, &team)
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", team.Key, team.Name)
			}
		},
	}
	cmd.Flags().BoolVarP(&jsonOut, "json", "j", false, "Output as JSON")
	return cmd
}

func resolveTeamArg(arg string) (*teams.Team, error) {
	key := strings.ToUpper(arg)
	if key == "" {
		if t, ok := config.GetOption("team_id"); ok {
			key = strings.ToUpper(t)
		}
	}
	if key == "" {
		return nil, errors.NewValidationError(
			"Could not determine team key from directory name",
			errors.WithSuggestion("Please specify a team key, name, or ID as an argument."),
		)
	}
	return teams.Resolve(context.Background(), key)
}

func newMembersCommand() *cobra.Command {
	var all, jsonOut bool
	cmd := &cobra.Command{
		Use:   "members [team]",
		Short: "List team members",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			arg := ""
			if len(args) > 0 {
				arg = args[0]
			}
			team, err := resolveTeamArg(arg)
			if err != nil {
				errors.HandleError(err, "Failed to list team members")
				os.Exit(1)
			}
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to list team members")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), teamMembersQuery, map[string]any{
				"teamId":          team.ID,
				"includeDisabled": all,
				"first":           100,
			})
			if err != nil {
				errors.HandleError(err, "Failed to list team members")
				os.Exit(1)
			}
			var parsed struct {
				Team *struct {
					Members struct {
						Nodes    []json.RawMessage `json:"nodes"`
						PageInfo json.RawMessage   `json:"pageInfo"`
					} `json:"members"`
				} `json:"team"`
			}
			_ = json.Unmarshal(data, &parsed)
			if parsed.Team == nil {
				errors.HandleError(errors.NewNotFoundError("Team", team.Key), "Failed to list team members")
				os.Exit(1)
			}
			nodes := parsed.Team.Members.Nodes
			if !all {
				filtered := nodes[:0]
				for _, raw := range nodes {
					var m struct {
						Active bool `json:"active"`
					}
					_ = json.Unmarshal(raw, &m)
					if m.Active {
						filtered = append(filtered, raw)
					}
				}
				nodes = filtered
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(map[string]any{"nodes": nodes, "pageInfo": parsed.Team.Members.PageInfo})
				return
			}
			for _, raw := range nodes {
				var m struct {
					DisplayName string `json:"displayName"`
					Email       string `json:"email"`
				}
				_ = json.Unmarshal(raw, &m)
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t<%s>\n", m.DisplayName, m.Email)
			}
		},
	}
	cmd.Flags().BoolVarP(&all, "all", "a", false, "Include inactive members")
	cmd.Flags().BoolVarP(&jsonOut, "json", "j", false, "Output as JSON")
	return cmd
}

func newStatesCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "states [team]",
		Short: "List workflow states",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			arg := ""
			if len(args) > 0 {
				arg = args[0]
			}
			team, err := resolveTeamArg(arg)
			if err != nil {
				errors.HandleError(err, "Failed to list states")
				os.Exit(1)
			}
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to list states")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), teamStatesQuery, map[string]any{"teamId": team.ID})
			if err != nil {
				errors.HandleError(err, "Failed to list states")
				os.Exit(1)
			}
			var parsed struct {
				Team *struct {
					States struct {
						Nodes []json.RawMessage `json:"nodes"`
					} `json:"states"`
				} `json:"team"`
			}
			_ = json.Unmarshal(data, &parsed)
			if parsed.Team == nil {
				errors.HandleError(errors.NewNotFoundError("Team", team.Key), "Failed to list states")
				os.Exit(1)
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(map[string]any{"nodes": parsed.Team.States.Nodes})
				return
			}
			for _, raw := range parsed.Team.States.Nodes {
				var s struct {
					Name string `json:"name"`
					Type string `json:"type"`
				}
				_ = json.Unmarshal(raw, &s)
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", s.Type, s.Name)
			}
		},
	}
	cmd.Flags().BoolVarP(&jsonOut, "json", "j", false, "Output as JSON")
	return cmd
}

func newIDCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "id [team]",
		Short: "Print team id",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			arg := ""
			if len(args) > 0 {
				arg = args[0]
			}
			team, err := resolveTeamArg(arg)
			if err != nil {
				errors.HandleError(err, "Failed to get team id")
				os.Exit(1)
			}
			fmt.Fprintln(cmd.OutOrStdout(), team.ID)
		},
	}
}

func newCreateCommand() *cobra.Command {
	var name, description, key string
	var isPrivate bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a team",
		Run: func(cmd *cobra.Command, args []string) {
			if name == "" {
				errors.HandleError(
					errors.NewValidationError(
						"Team name is required when not using interactive mode",
						errors.WithSuggestion("Use --name or run without any flags for interactive mode."),
					),
					"Failed to create team",
				)
				os.Exit(1)
			}
			input := gql.TeamCreateInput{Name: name}
			if description != "" {
				input.Description = &description
			}
			if key != "" {
				input.Key = &key
			}
			if isPrivate {
				t := true
				input.Private = &t
			}
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to create team")
				os.Exit(1)
			}
			resp, err := gql.CreateTeam(context.Background(), client, input)
			if err != nil {
				errors.HandleError(err, "Failed to create team")
				os.Exit(1)
			}
			if !resp.TeamCreate.Success {
				errors.HandleError(errors.NewCliError("Team creation failed"), "Failed to create team")
				os.Exit(1)
			}
			t := resp.TeamCreate.Team
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Created team %s: %s\n", t.Key, t.Name)
		},
	}
	cmd.Flags().StringVarP(&name, "name", "n", "", "Name of the team")
	cmd.Flags().StringVarP(&description, "description", "d", "", "Description of the team")
	cmd.Flags().StringVarP(&key, "key", "k", "", "Team key")
	cmd.Flags().BoolVar(&isPrivate, "private", false, "Make the team private")
	return cmd
}

func newDeleteCommand() *cobra.Command {
	var force bool
	var moveIssues string
	cmd := &cobra.Command{
		Use:   "delete <team>",
		Short: "Delete a team",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			team, err := teams.Resolve(context.Background(), args[0])
			if err != nil {
				errors.HandleError(err, "Failed to delete team")
				os.Exit(1)
			}
			ok, err := prompt.ConfirmWithFlag(
				fmt.Sprintf(`Are you sure you want to delete team "%s: %s"?`, team.Key, team.Name),
				force,
				"--force",
			)
			if err != nil {
				errors.HandleError(err, "Failed to delete team")
				os.Exit(1)
			}
			if !ok {
				fmt.Fprintln(cmd.OutOrStdout(), "Delete cancelled.")
				return
			}
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to delete team")
				os.Exit(1)
			}
			if moveIssues != "" {
				target, err := teams.Resolve(context.Background(), moveIssues)
				if err != nil {
					errors.HandleError(err, "Failed to delete team")
					os.Exit(1)
				}
				if target.ID == team.ID {
					errors.HandleError(
						errors.NewValidationError("Cannot move issues to the same team"),
						"Failed to delete team",
					)
					os.Exit(1)
				}
				if err := moveTeamIssues(client, team.ID, target.ID); err != nil {
					errors.HandleError(err, "Failed to delete team")
					os.Exit(1)
				}
			}
			data, err := client.RequestRaw(context.Background(), `
mutation DeleteTeam($id: String!) {
  teamDelete(id: $id) { success }
}`, map[string]any{"id": team.ID})
			if err != nil {
				errors.HandleError(err, "Failed to delete team")
				os.Exit(1)
			}
			var parsed struct {
				TeamDelete struct {
					Success bool `json:"success"`
				} `json:"teamDelete"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil || !parsed.TeamDelete.Success {
				errors.HandleError(errors.NewCliError("Failed to delete team"), "Failed to delete team")
				os.Exit(1)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Successfully deleted team: %s: %s\n", team.Key, team.Name)
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "y", false, "Skip confirmation prompt")
	cmd.Flags().StringVar(&moveIssues, "move-issues", "", "Move all issues to another team before deletion")
	return cmd
}

func moveTeamIssues(client *graphql.Client, sourceID, targetID string) error {
	var after *string
	moved := 0
	for {
		vars := map[string]any{"teamId": sourceID, "first": 100}
		if after != nil {
			vars["after"] = *after
		}
		data, err := client.RequestRaw(context.Background(), `
query GetTeamIssuesForMove($teamId: String!, $first: Int, $after: String) {
  team(id: $teamId) {
    issues(first: $first, after: $after) {
      nodes { id }
      pageInfo { hasNextPage endCursor }
    }
  }
}`, vars)
		if err != nil {
			return err
		}
		var parsed struct {
			Team *struct {
				Issues struct {
					Nodes []struct {
						ID string `json:"id"`
					} `json:"nodes"`
					PageInfo struct {
						HasNextPage bool    `json:"hasNextPage"`
						EndCursor   *string `json:"endCursor"`
					} `json:"pageInfo"`
				} `json:"issues"`
			} `json:"team"`
		}
		if err := json.Unmarshal(data, &parsed); err != nil {
			return err
		}
		if parsed.Team == nil {
			break
		}
		for _, iss := range parsed.Team.Issues.Nodes {
			_, err := client.RequestRaw(context.Background(), `
mutation MoveIssueToTeam($id: String!, $teamId: String!) {
  issueUpdate(id: $id, input: { teamId: $teamId }) { success }
}`, map[string]any{"id": iss.ID, "teamId": targetID})
			if err != nil {
				return err
			}
			moved++
		}
		if !parsed.Team.Issues.PageInfo.HasNextPage {
			break
		}
		after = parsed.Team.Issues.PageInfo.EndCursor
		if after == nil {
			break
		}
	}
	fmt.Printf("✓ Moved %d issue(s) to target team\n", moved)
	return nil
}

func newAutolinksCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "autolinks",
		Short: "Configure GitHub repository autolinks for Linear issues",
		Run: func(cmd *cobra.Command, args []string) {
			teamKey, ok := config.GetOption("team_id")
			if !ok || teamKey == "" {
				errors.HandleError(
					errors.NewValidationError(
						"Could not determine team id from directory name",
						errors.WithSuggestion("Run `linear config` to set a team."),
					),
					"Failed to configure autolinks",
				)
				os.Exit(1)
			}
			workspace, ok := config.GetOption("workspace")
			if !ok || workspace == "" {
				errors.HandleError(
					errors.NewValidationError(
						"workspace is not set via command line, configuration file, or environment",
					),
					"Failed to configure autolinks",
				)
				os.Exit(1)
			}
			urlTemplate := fmt.Sprintf("https://linear.app/%s/issue/%s-<num>", workspace, teamKey)
			gh := exec.Command("gh", "api", "repos/{owner}/{repo}/autolinks",
				"-f", fmt.Sprintf("key_prefix=%s-", teamKey),
				"-f", fmt.Sprintf("url_template=%s", urlTemplate),
			)
			gh.Stdout = cmd.OutOrStdout()
			gh.Stderr = cmd.ErrOrStderr()
			gh.Stdin = os.Stdin
			if err := gh.Run(); err != nil {
				errors.HandleError(errors.NewCliError("Failed to configure autolinks"), "Failed to configure autolinks")
				os.Exit(1)
			}
		},
	}
}

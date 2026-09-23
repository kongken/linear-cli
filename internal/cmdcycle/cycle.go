package cmdcycle

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/kongken/linear-cli/internal/ids"
	"github.com/kongken/linear-cli/internal/config"
	"github.com/kongken/linear-cli/internal/errors"
	"github.com/kongken/linear-cli/internal/graphql"
	"github.com/kongken/linear-cli/internal/teams"
	"github.com/spf13/cobra"
)

const teamCyclesQuery = `
query GetTeamCycles($teamId: String!, $first: Int, $after: String) {
  team(id: $teamId) {
    id
    name
    cycles(first: $first, after: $after) {
      nodes {
        id number name startsAt endsAt completedAt
        isActive isFuture isPast
      }
      pageInfo { hasNextPage endCursor }
    }
  }
}
`

// New returns the cycle command group.
func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cycle",
		Short: "Manage Linear cycles",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newListCommand(), newViewCommand())
	return cmd
}

func newViewCommand() *cobra.Command {
	var team string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:     "view <cycleRef>",
		Aliases: []string{"v"},
		Short:   "View cycle details",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			teamKey := strings.ToUpper(team)
			if teamKey == "" {
				if t, ok := config.GetOption("team_id"); ok {
					teamKey = strings.ToUpper(t)
				}
			}
			if teamKey == "" {
				errors.HandleError(
					errors.NewValidationError("Could not determine team key from directory name or team flag"),
					"Failed to view cycle",
				)
				os.Exit(1)
			}
			resolved, err := teams.Resolve(context.Background(), teamKey)
			if err != nil {
				errors.HandleError(err, "Failed to view cycle")
				os.Exit(1)
			}
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to view cycle")
				os.Exit(1)
			}
			cycleID, err := resolveCycleRef(client, resolved.ID, args[0])
			if err != nil {
				errors.HandleError(err, "Failed to view cycle")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), `
query GetCycleDetails($id: String!) {
  cycle(id: $id) {
    id number name description startsAt endsAt completedAt
    isActive isFuture isPast createdAt updatedAt
    team { id key name }
    issues {
      nodes {
        id identifier title
        state { name type }
      }
      pageInfo { hasNextPage endCursor }
    }
  }
}`, map[string]any{"id": cycleID})
			if err != nil {
				errors.HandleError(err, "Failed to view cycle")
				os.Exit(1)
			}
			var parsed struct {
				Cycle *struct {
					Number      int     `json:"number"`
					Name        *string `json:"name"`
					StartsAt    string  `json:"startsAt"`
					EndsAt      string  `json:"endsAt"`
					CompletedAt *string `json:"completedAt"`
					IsActive    bool    `json:"isActive"`
					IsFuture    bool    `json:"isFuture"`
					IsPast      bool    `json:"isPast"`
					Team        struct {
						Key  string `json:"key"`
						Name string `json:"name"`
					} `json:"team"`
					Issues struct {
						Nodes []struct {
							Identifier string `json:"identifier"`
							Title      string `json:"title"`
							State      struct {
								Name string `json:"name"`
							} `json:"state"`
						} `json:"nodes"`
					} `json:"issues"`
				} `json:"cycle"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil || parsed.Cycle == nil {
				errors.HandleError(errors.NewNotFoundError("Cycle", args[0]), "Failed to view cycle")
				os.Exit(1)
			}
			c := parsed.Cycle
			if jsonOut {
				var pretty any
				_ = json.Unmarshal(data, &pretty)
				if m, ok := pretty.(map[string]any); ok {
					pretty = m["cycle"]
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(pretty)
				return
			}
			title := fmt.Sprintf("Cycle %d", c.Number)
			if c.Name != nil && *c.Name != "" {
				title = *c.Name
			}
			fmt.Fprintf(cmd.OutOrStdout(), "# %s\n\n", title)
			fmt.Fprintf(cmd.OutOrStdout(), "**Number:** %d\n", c.Number)
			start := c.StartsAt
			if len(start) >= 10 {
				start = start[:10]
			}
			end := c.EndsAt
			if len(end) >= 10 {
				end = end[:10]
			}
			fmt.Fprintf(cmd.OutOrStdout(), "**Start:** %s\n", start)
			fmt.Fprintf(cmd.OutOrStdout(), "**End:** %s\n", end)
			status := "Unknown"
			switch {
			case c.IsActive:
				status = "Active"
			case c.IsFuture:
				status = "Upcoming"
			case c.CompletedAt != nil:
				status = "Completed"
			case c.IsPast:
				status = "Past"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "**Status:** %s\n", status)
			fmt.Fprintf(cmd.OutOrStdout(), "**Team:** %s (%s)\n", c.Team.Name, c.Team.Key)
			if len(c.Issues.Nodes) > 0 {
				fmt.Fprint(cmd.OutOrStdout(), "\n## Issues\n\n")
				for _, iss := range c.Issues.Nodes {
					fmt.Fprintf(cmd.OutOrStdout(), "- %s: %s (%s)\n", iss.Identifier, iss.Title, iss.State.Name)
				}
			}
		},
	}
	cmd.Flags().StringVar(&team, "team", "", "Team key, name, or ID")
	cmd.Flags().BoolVarP(&jsonOut, "json", "j", false, "Output as JSON")
	return cmd
}

func resolveCycleRef(client *graphql.Client, teamID, ref string) (string, error) {
	// UUID passthrough
	if ids.IsUUID(ref) {
		return ref, nil
	}
	data, err := client.RequestRaw(context.Background(), teamCyclesQuery, map[string]any{
		"teamId": teamID, "first": 100,
	})
	if err != nil {
		return "", err
	}
	var parsed struct {
		Team *struct {
			Cycles struct {
				Nodes []struct {
					ID     string  `json:"id"`
					Number int     `json:"number"`
					Name   *string `json:"name"`
				} `json:"nodes"`
			} `json:"cycles"`
		} `json:"team"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil || parsed.Team == nil {
		return "", errors.NewNotFoundError("Cycle", ref)
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

func newListCommand() *cobra.Command {
	var team string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List cycles for a team",
		Run: func(cmd *cobra.Command, args []string) {
			teamKey := strings.ToUpper(team)
			if teamKey == "" {
				if t, ok := config.GetOption("team_id"); ok {
					teamKey = strings.ToUpper(t)
				}
			}
			if teamKey == "" {
				errors.HandleError(
					errors.NewValidationError("Could not determine team key from directory name or team flag"),
					"Failed to list cycles",
				)
				os.Exit(1)
			}
			resolved, err := teams.Resolve(context.Background(), teamKey)
			if err != nil {
				errors.HandleError(err, "Failed to list cycles")
				os.Exit(1)
			}
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to list cycles")
				os.Exit(1)
			}
			var all []json.RawMessage
			var pageInfo json.RawMessage
			var after *string
			for {
				vars := map[string]any{"teamId": resolved.ID, "first": 50}
				if after != nil {
					vars["after"] = *after
				}
				data, err := client.RequestRaw(context.Background(), teamCyclesQuery, vars)
				if err != nil {
					errors.HandleError(err, "Failed to list cycles")
					os.Exit(1)
				}
				var parsed struct {
					Team *struct {
						Cycles struct {
							Nodes    []json.RawMessage `json:"nodes"`
							PageInfo struct {
								HasNextPage bool    `json:"hasNextPage"`
								EndCursor   *string `json:"endCursor"`
							} `json:"pageInfo"`
						} `json:"cycles"`
					} `json:"team"`
				}
				if err := json.Unmarshal(data, &parsed); err != nil {
					errors.HandleError(err, "Failed to list cycles")
					os.Exit(1)
				}
				if parsed.Team == nil {
					errors.HandleError(errors.NewNotFoundError("Team", teamKey), "Failed to list cycles")
					os.Exit(1)
				}
				all = append(all, parsed.Team.Cycles.Nodes...)
				pi, _ := json.Marshal(parsed.Team.Cycles.PageInfo)
				pageInfo = pi
				if !parsed.Team.Cycles.PageInfo.HasNextPage || parsed.Team.Cycles.PageInfo.EndCursor == nil {
					break
				}
				after = parsed.Team.Cycles.PageInfo.EndCursor
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(map[string]any{"nodes": all, "pageInfo": json.RawMessage(pageInfo)})
				return
			}
			for _, raw := range all {
				var c struct {
					Number   int    `json:"number"`
					Name     string `json:"name"`
					IsActive bool   `json:"isActive"`
				}
				_ = json.Unmarshal(raw, &c)
				status := ""
				if c.IsActive {
					status = "active"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%d\t%s\t%s\n", c.Number, c.Name, status)
			}
		},
	}
	cmd.Flags().StringVar(&team, "team", "", "Team key, name, or ID")
	cmd.Flags().BoolVarP(&jsonOut, "json", "j", false, "Output as JSON")
	return cmd
}

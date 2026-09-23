package cmdprojectupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/kongken/linear-cli/internal/ids"
	"github.com/kongken/linear-cli/internal/editor"
	"github.com/kongken/linear-cli/internal/errors"
	"github.com/kongken/linear-cli/internal/graphql"
	"github.com/kongken/linear-cli/internal/prompt"
	"github.com/spf13/cobra"
)

const listProjectUpdatesQuery = `
query ListProjectUpdates($id: String!, $first: Int) {
  project(id: $id) {
    name
    slugId
    projectUpdates(first: $first) {
      nodes {
        id body health url createdAt
        user { name displayName }
      }
      pageInfo { hasNextPage endCursor }
    }
  }
}
`

const resolveProjectQuery = `
query ResolveProjectForUpdates($name: String!) {
  projects(filter: { name: { eqIgnoreCase: $name } }, first: 5) {
    nodes { id name slugId }
  }
}
`

const createProjectUpdateMutation = `
mutation ProjectUpdateCreate($input: ProjectUpdateCreateInput!) {
  projectUpdateCreate(input: $input) {
    success
    projectUpdate { id url body health }
  }
}
`

// New returns the project-update command group.
func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project-update",
		Short: "Manage project updates",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newListCommand(), newCreateCommand())
	return cmd
}

func resolveProjectID(client *graphql.Client, ref string) (string, error) {
	if ids.IsUUID(ref) {
		return ref, nil
	}
	data, err := client.RequestRaw(context.Background(), resolveProjectQuery, map[string]any{"name": ref})
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

func newListCommand() *cobra.Command {
	var (
		jsonOut bool
		limit   int
	)
	cmd := &cobra.Command{
		Use:     "list <projectId>",
		Aliases: []string{"l"},
		Short:   "List status updates for a project",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to list project updates")
				os.Exit(1)
			}
			id, err := resolveProjectID(client, args[0])
			if err != nil {
				errors.HandleError(err, "Failed to list project updates")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), listProjectUpdatesQuery, map[string]any{
				"id": id, "first": limit,
			})
			if err != nil {
				errors.HandleError(err, "Failed to list project updates")
				os.Exit(1)
			}
			var parsed struct {
				Project *struct {
					Name           string `json:"name"`
					ProjectUpdates struct {
						Nodes []json.RawMessage `json:"nodes"`
					} `json:"projectUpdates"`
				} `json:"project"`
			}
			_ = json.Unmarshal(data, &parsed)
			if parsed.Project == nil {
				errors.HandleError(errors.NewNotFoundError("Project", args[0]), "Failed to list project updates")
				os.Exit(1)
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(parsed.Project)
				return
			}
			if len(parsed.Project.ProjectUpdates.Nodes) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No status updates found for project: %s\n", parsed.Project.Name)
				return
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Status updates for: %s\n\n", parsed.Project.Name)
			for _, raw := range parsed.Project.ProjectUpdates.Nodes {
				var u struct {
					Health string `json:"health"`
					Body   string `json:"body"`
					URL    string `json:"url"`
					User   *struct {
						DisplayName string `json:"displayName"`
					} `json:"user"`
				}
				_ = json.Unmarshal(raw, &u)
				who := ""
				if u.User != nil {
					who = u.User.DisplayName
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n%s\n\n", u.Health, who, u.URL, u.Body)
			}
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	cmd.Flags().IntVar(&limit, "limit", 10, "Limit results")
	return cmd
}

func newCreateCommand() *cobra.Command {
	var body, health string
	cmd := &cobra.Command{
		Use:   "create <projectId>",
		Short: "Create a project status update",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if body == "" {
				if !prompt.IsInteractive() {
					errors.HandleError(
						errors.NewValidationError("body is required", errors.WithSuggestion("Pass --body")),
						"Failed to create project update",
					)
					os.Exit(1)
				}
				fmt.Fprintln(cmd.ErrOrStderr(), "Opening editor for update content...")
				edited, err := editor.Open()
				if err != nil {
					errors.HandleError(err, "Failed to create project update")
					os.Exit(1)
				}
				if edited == "" {
					errors.HandleError(
						errors.NewValidationError("body is required", errors.WithSuggestion("Pass --body or provide content in the editor")),
						"Failed to create project update",
					)
					os.Exit(1)
				}
				body = edited
			}
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to create project update")
				os.Exit(1)
			}
			id, err := resolveProjectID(client, args[0])
			if err != nil {
				errors.HandleError(err, "Failed to create project update")
				os.Exit(1)
			}
			input := map[string]any{"projectId": id, "body": body}
			if health != "" {
				input["health"] = health
			}
			data, err := client.RequestRaw(context.Background(), createProjectUpdateMutation, map[string]any{"input": input})
			if err != nil {
				errors.HandleError(err, "Failed to create project update")
				os.Exit(1)
			}
			var parsed struct {
				ProjectUpdateCreate struct {
					Success       bool `json:"success"`
					ProjectUpdate *struct {
						URL string `json:"url"`
					} `json:"projectUpdate"`
				} `json:"projectUpdateCreate"`
			}
			_ = json.Unmarshal(data, &parsed)
			if !parsed.ProjectUpdateCreate.Success || parsed.ProjectUpdateCreate.ProjectUpdate == nil {
				errors.HandleError(errors.NewCliError("Failed to create project update"), "Failed to create project update")
				os.Exit(1)
			}
			fmt.Fprintln(cmd.OutOrStdout(), parsed.ProjectUpdateCreate.ProjectUpdate.URL)
		},
	}
	cmd.Flags().StringVarP(&body, "body", "b", "", "Update body")
	cmd.Flags().StringVar(&health, "health", "", "Health (onTrack, atRisk, offTrack)")
	return cmd
}

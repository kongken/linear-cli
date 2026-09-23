package cmdmilestone

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/kongken/linear-cli/internal/ids"
	"github.com/kongken/linear-cli/internal/errors"
	"github.com/kongken/linear-cli/internal/graphql"
	"github.com/kongken/linear-cli/internal/prompt"
	"github.com/spf13/cobra"
)

const projectMilestonesQuery = `
query GetProjectMilestones($projectId: String!, $first: Int, $after: String) {
  project(id: $projectId) {
    id
    name
    projectMilestones(first: $first, after: $after) {
      nodes {
        id name targetDate sortOrder
        project { id name }
      }
      pageInfo { hasNextPage endCursor }
    }
  }
}
`

const resolveProjectQuery = `
query ResolveProject($name: String!) {
  projects(filter: { name: { eqIgnoreCase: $name } }, first: 5) {
    nodes { id name slugId }
  }
}
`

const getMilestoneQuery = `
query GetMilestoneDetails($id: String!, $first: Int!, $after: String) {
  projectMilestone(id: $id) {
    id name description targetDate sortOrder createdAt updatedAt
    project { id name slugId url }
    issues(first: $first, after: $after) {
      nodes {
        id identifier title
        state { name type }
      }
      pageInfo { hasNextPage endCursor }
    }
  }
}
`

// New returns the milestone command group.
func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "milestone",
		Short: "Manage Linear milestones",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		newListCommand(),
		newViewCommand(),
		newCreateCommand(),
		newUpdateCommand(),
		newDeleteCommand(),
	)
	return cmd
}

func resolveProjectID(client *graphql.Client, project string) (string, error) {
	if ids.IsUUID(project) {
		return project, nil
	}
	data, err := client.RequestRaw(context.Background(), resolveProjectQuery, map[string]any{"name": project})
	if err != nil {
		return "", err
	}
	var resolved struct {
		Projects struct {
			Nodes []struct {
				ID string `json:"id"`
			} `json:"nodes"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(data, &resolved); err != nil {
		return "", err
	}
	if len(resolved.Projects.Nodes) == 0 {
		return "", errors.NewNotFoundError("Project", project)
	}
	return resolved.Projects.Nodes[0].ID, nil
}

func newListCommand() *cobra.Command {
	var project string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List milestones for a project",
		Run: func(cmd *cobra.Command, args []string) {
			if project == "" {
				errors.HandleError(
					errors.NewValidationError("project is required", errors.WithSuggestion("Pass --project")),
					"Failed to list milestones",
				)
				os.Exit(1)
			}
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to list milestones")
				os.Exit(1)
			}
			projectID, err := resolveProjectID(client, project)
			if err != nil {
				errors.HandleError(err, "Failed to list milestones")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), projectMilestonesQuery, map[string]any{
				"projectId": projectID,
				"first":     100,
			})
			if err != nil {
				errors.HandleError(err, "Failed to list milestones")
				os.Exit(1)
			}
			var parsed struct {
				Project *struct {
					ProjectMilestones struct {
						Nodes    []json.RawMessage `json:"nodes"`
						PageInfo json.RawMessage   `json:"pageInfo"`
					} `json:"projectMilestones"`
				} `json:"project"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil {
				errors.HandleError(err, "Failed to list milestones")
				os.Exit(1)
			}
			if parsed.Project == nil {
				errors.HandleError(errors.NewNotFoundError("Project", project), "Failed to list milestones")
				os.Exit(1)
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(map[string]any{
					"nodes":    parsed.Project.ProjectMilestones.Nodes,
					"pageInfo": parsed.Project.ProjectMilestones.PageInfo,
				})
				return
			}
			for _, raw := range parsed.Project.ProjectMilestones.Nodes {
				var m struct {
					Name       string  `json:"name"`
					TargetDate *string `json:"targetDate"`
				}
				_ = json.Unmarshal(raw, &m)
				date := ""
				if m.TargetDate != nil {
					date = *m.TargetDate
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", m.Name, date)
			}
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "Project (UUID, slug ID, or name)")
	cmd.Flags().BoolVarP(&jsonOut, "json", "j", false, "Output as JSON")
	_ = cmd.MarkFlagRequired("project")
	return cmd
}

func newViewCommand() *cobra.Command {
	var jsonOut bool
	var all bool
	cmd := &cobra.Command{
		Use:     "view <milestone>",
		Aliases: []string{"v"},
		Short:   "View milestone details",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to fetch milestone details")
				os.Exit(1)
			}
			milestoneID := args[0]
			data, err := client.RequestRaw(context.Background(), getMilestoneQuery, map[string]any{
				"id": milestoneID, "first": 50,
			})
			if err != nil {
				errors.HandleError(err, "Failed to fetch milestone details")
				os.Exit(1)
			}
			var parsed struct {
				ProjectMilestone *struct {
					ID          string  `json:"id"`
					Name        string  `json:"name"`
					Description *string `json:"description"`
					TargetDate  *string `json:"targetDate"`
					Project     struct {
						Name   string `json:"name"`
						SlugID string `json:"slugId"`
						URL    string `json:"url"`
					} `json:"project"`
					Issues struct {
						Nodes []struct {
							Identifier string `json:"identifier"`
							Title      string `json:"title"`
							State      struct {
								Name string `json:"name"`
								Type string `json:"type"`
							} `json:"state"`
						} `json:"nodes"`
						PageInfo struct {
							HasNextPage bool `json:"hasNextPage"`
						} `json:"pageInfo"`
					} `json:"issues"`
				} `json:"projectMilestone"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil || parsed.ProjectMilestone == nil {
				errors.HandleError(errors.NewNotFoundError("Milestone", args[0]), "Failed to fetch milestone details")
				os.Exit(1)
			}
			m := parsed.ProjectMilestone
			if jsonOut {
				var pretty any
				_ = json.Unmarshal(data, &pretty)
				if mp, ok := pretty.(map[string]any); ok {
					pretty = mp["projectMilestone"]
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(pretty)
				return
			}
			fmt.Fprintf(cmd.OutOrStdout(), "# %s\n\n", m.Name)
			fmt.Fprintf(cmd.OutOrStdout(), "**ID:** %s\n", m.ID)
			if m.TargetDate != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "**Target Date:** %s\n", *m.TargetDate)
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "**Target Date:** Not set")
			}
			fmt.Fprintf(cmd.OutOrStdout(), "**Project:** %s (%s)\n", m.Project.Name, m.Project.SlugID)
			fmt.Fprintf(cmd.OutOrStdout(), "**Project URL:** %s\n", m.Project.URL)
			if m.Description != nil && *m.Description != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "\n## Description\n\n%s\n", *m.Description)
			}
			issues := m.Issues.Nodes
			if len(issues) > 0 {
				fmt.Fprint(cmd.OutOrStdout(), "\n## Issues\n\n")
				limit := len(issues)
				if !all && limit > 10 {
					limit = 10
				}
				for _, iss := range issues[:limit] {
					fmt.Fprintf(cmd.OutOrStdout(), "- %s: %s (%s)\n", iss.Identifier, iss.Title, iss.State.Name)
				}
				if !all && (len(issues) > 10 || m.Issues.PageInfo.HasNextPage) {
					fmt.Fprint(cmd.OutOrStdout(), "\n_Use --all for the full issue list._\n")
				}
			} else {
				fmt.Fprint(cmd.OutOrStdout(), "\n_No issues in this milestone yet._\n")
			}
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "List every attached issue")
	cmd.Flags().BoolVarP(&jsonOut, "json", "j", false, "Output as JSON")
	return cmd
}

func newCreateCommand() *cobra.Command {
	var project, name, description, targetDate string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a project milestone",
		Run: func(cmd *cobra.Command, args []string) {
			if project == "" || name == "" {
				errors.HandleError(
					errors.NewValidationError("--project and --name are required"),
					"Failed to create milestone",
				)
				os.Exit(1)
			}
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to create milestone")
				os.Exit(1)
			}
			projectID, err := resolveProjectID(client, project)
			if err != nil {
				errors.HandleError(err, "Failed to create milestone")
				os.Exit(1)
			}
			input := map[string]any{"projectId": projectID, "name": name}
			if description != "" {
				input["description"] = description
			}
			if targetDate != "" {
				input["targetDate"] = targetDate
			}
			data, err := client.RequestRaw(context.Background(), `
mutation CreateProjectMilestone($input: ProjectMilestoneCreateInput!) {
  projectMilestoneCreate(input: $input) {
    success
    projectMilestone {
      id name targetDate
      project { name }
    }
  }
}`, map[string]any{"input": input})
			if err != nil {
				errors.HandleError(err, "Failed to create milestone")
				os.Exit(1)
			}
			var parsed struct {
				ProjectMilestoneCreate struct {
					Success          bool `json:"success"`
					ProjectMilestone *struct {
						ID         string  `json:"id"`
						Name       string  `json:"name"`
						TargetDate *string `json:"targetDate"`
						Project    struct {
							Name string `json:"name"`
						} `json:"project"`
					} `json:"projectMilestone"`
				} `json:"projectMilestoneCreate"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil || !parsed.ProjectMilestoneCreate.Success || parsed.ProjectMilestoneCreate.ProjectMilestone == nil {
				errors.HandleError(errors.NewCliError("Failed to create milestone"), "Failed to create milestone")
				os.Exit(1)
			}
			m := parsed.ProjectMilestoneCreate.ProjectMilestone
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Created milestone: %s\n", m.Name)
			fmt.Fprintf(cmd.OutOrStdout(), "  ID: %s\n", m.ID)
			if m.TargetDate != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "  Target Date: %s\n", *m.TargetDate)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  Project: %s\n", m.Project.Name)
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "Project (UUID, slug ID, or name)")
	cmd.Flags().StringVar(&name, "name", "", "Milestone name")
	cmd.Flags().StringVar(&description, "description", "", "Milestone description")
	cmd.Flags().StringVar(&targetDate, "target-date", "", "Target date (YYYY-MM-DD)")
	_ = cmd.MarkFlagRequired("project")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func newUpdateCommand() *cobra.Command {
	var name, description, targetDate string
	cmd := &cobra.Command{
		Use:   "update <milestoneId>",
		Short: "Update a milestone",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if name == "" && description == "" && targetDate == "" {
				errors.HandleError(
					errors.NewValidationError("At least one update option required", errors.WithSuggestion("Use --name, --description, or --target-date")),
					"Failed to update milestone",
				)
				os.Exit(1)
			}
			input := map[string]any{}
			if name != "" {
				input["name"] = name
			}
			if description != "" {
				input["description"] = description
			}
			if targetDate != "" {
				input["targetDate"] = targetDate
			}
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to update milestone")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), `
mutation UpdateProjectMilestone($id: String!, $input: ProjectMilestoneUpdateInput!) {
  projectMilestoneUpdate(id: $id, input: $input) {
    success
    projectMilestone { id name }
  }
}`, map[string]any{"id": args[0], "input": input})
			if err != nil {
				errors.HandleError(err, "Failed to update milestone")
				os.Exit(1)
			}
			var parsed struct {
				ProjectMilestoneUpdate struct {
					Success          bool `json:"success"`
					ProjectMilestone *struct {
						Name string `json:"name"`
					} `json:"projectMilestone"`
				} `json:"projectMilestoneUpdate"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil || !parsed.ProjectMilestoneUpdate.Success {
				errors.HandleError(errors.NewCliError("Failed to update milestone"), "Failed to update milestone")
				os.Exit(1)
			}
			if parsed.ProjectMilestoneUpdate.ProjectMilestone != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "✓ Updated milestone: %s\n", parsed.ProjectMilestoneUpdate.ProjectMilestone.Name)
			}
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Milestone name")
	cmd.Flags().StringVar(&description, "description", "", "Milestone description")
	cmd.Flags().StringVar(&targetDate, "target-date", "", "Target date (YYYY-MM-DD)")
	return cmd
}

func newDeleteCommand() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "delete <milestoneId>",
		Short: "Delete a milestone",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ok, err := prompt.Confirm(fmt.Sprintf("Are you sure you want to delete milestone %s?", args[0]), force)
			if err != nil {
				errors.HandleError(err, "Failed to delete milestone")
				os.Exit(1)
			}
			if !ok {
				fmt.Fprintln(cmd.OutOrStdout(), "Deletion canceled")
				return
			}
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to delete milestone")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), `
mutation DeleteProjectMilestone($id: String!) {
  projectMilestoneDelete(id: $id) { success }
}`, map[string]any{"id": args[0]})
			if err != nil {
				errors.HandleError(err, "Failed to delete milestone")
				os.Exit(1)
			}
			var parsed struct {
				ProjectMilestoneDelete struct {
					Success bool `json:"success"`
				} `json:"projectMilestoneDelete"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil || !parsed.ProjectMilestoneDelete.Success {
				errors.HandleError(errors.NewCliError("Failed to delete milestone"), "Failed to delete milestone")
				os.Exit(1)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Deleted milestone: %s\n", args[0])
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation")
	return cmd
}

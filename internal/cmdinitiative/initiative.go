package cmdinitiative

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/kongken/linear-cli/internal/errors"
	"github.com/kongken/linear-cli/internal/graphql"
	"github.com/kongken/linear-cli/internal/prompt"
	"github.com/spf13/cobra"
)

const getInitiativesQuery = `
query GetInitiatives($filter: InitiativeFilter, $includeArchived: Boolean) {
  initiatives(filter: $filter, includeArchived: $includeArchived) {
    nodes {
      id slugId name description status targetDate health url archivedAt
      owner { id displayName }
      projects { nodes { id name status { name } } }
    }
    pageInfo { hasNextPage endCursor }
  }
}
`

const getInitiativeDetailsQuery = `
query GetInitiativeDetails($id: String!) {
  initiative(id: $id) {
    id slugId name description status targetDate health color icon url
    archivedAt createdAt updatedAt
    owner { id name displayName }
    projects {
      nodes {
        id slugId name
        status { name type }
      }
    }
  }
}
`

var uuidRE = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// New returns the initiative command group.
func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "initiative",
		Short: "Manage Linear initiatives",
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
		newArchiveCommand(),
		newUnarchiveCommand(),
		newAddProjectCommand(),
		newRemoveProjectCommand(),
		newCommentCommand(),
	)
	return cmd
}

func newListCommand() *cobra.Command {
	var status string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List initiatives",
		Run: func(cmd *cobra.Command, args []string) {
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to list initiatives")
				os.Exit(1)
			}
			vars := map[string]any{"includeArchived": false}
			if status != "" {
				mapped := map[string]string{
					"active": "Active", "planned": "Planned", "completed": "Completed",
				}
				apiStatus, ok := mapped[strings.ToLower(status)]
				if !ok {
					errors.HandleError(
						errors.NewValidationError(
							"invalid status",
							errors.WithSuggestion("Use active, planned, or completed"),
						),
						"Failed to list initiatives",
					)
					os.Exit(1)
				}
				vars["filter"] = map[string]any{"status": map[string]any{"eq": apiStatus}}
			}
			data, err := client.RequestRaw(context.Background(), getInitiativesQuery, vars)
			if err != nil {
				errors.HandleError(err, "Failed to list initiatives")
				os.Exit(1)
			}
			var parsed struct {
				Initiatives struct {
					Nodes    []json.RawMessage `json:"nodes"`
					PageInfo json.RawMessage   `json:"pageInfo"`
				} `json:"initiatives"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil {
				errors.HandleError(err, "Failed to list initiatives")
				os.Exit(1)
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(map[string]any{
					"nodes":    parsed.Initiatives.Nodes,
					"pageInfo": parsed.Initiatives.PageInfo,
				})
				return
			}
			for _, raw := range parsed.Initiatives.Nodes {
				var init struct {
					Name   string `json:"name"`
					Status string `json:"status"`
					URL    string `json:"url"`
				}
				_ = json.Unmarshal(raw, &init)
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", init.Status, init.Name, init.URL)
			}
		},
	}
	cmd.Flags().StringVarP(&status, "status", "s", "", "Filter by status (active, planned, completed)")
	cmd.Flags().BoolVarP(&jsonOut, "json", "j", false, "Output as JSON")
	return cmd
}

func resolveInitiativeID(client *graphql.Client, ref string) (string, error) {
	if uuidRE.MatchString(ref) {
		return ref, nil
	}
	for _, q := range []struct {
		query string
		key   string
	}{
		{`query($slugId: String!) { initiatives(filter: { slugId: { eq: $slugId } }) { nodes { id } } }`, "slugId"},
		{`query($name: String!) { initiatives(filter: { name: { eqIgnoreCase: $name } }) { nodes { id } } }`, "name"},
	} {
		data, err := client.RequestRaw(context.Background(), q.query, map[string]any{q.key: ref})
		if err != nil {
			continue
		}
		var parsed struct {
			Initiatives struct {
				Nodes []struct {
					ID string `json:"id"`
				} `json:"nodes"`
			} `json:"initiatives"`
		}
		if err := json.Unmarshal(data, &parsed); err != nil {
			continue
		}
		if len(parsed.Initiatives.Nodes) > 0 {
			return parsed.Initiatives.Nodes[0].ID, nil
		}
	}
	return "", errors.NewNotFoundError("Initiative", ref)
}

func newViewCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:     "view <initiativeId>",
		Aliases: []string{"v"},
		Short:   "View initiative details",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to fetch initiative details")
				os.Exit(1)
			}
			id, err := resolveInitiativeID(client, args[0])
			if err != nil {
				errors.HandleError(err, "Failed to fetch initiative details")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), getInitiativeDetailsQuery, map[string]any{"id": id})
			if err != nil {
				errors.HandleError(err, "Failed to fetch initiative details")
				os.Exit(1)
			}
			var parsed struct {
				Initiative *struct {
					Name        string  `json:"name"`
					SlugID      string  `json:"slugId"`
					Status      string  `json:"status"`
					URL         string  `json:"url"`
					Description *string `json:"description"`
					Health      *string `json:"health"`
					TargetDate  *string `json:"targetDate"`
					Icon        *string `json:"icon"`
					Owner       *struct {
						DisplayName string `json:"displayName"`
						Name        string `json:"name"`
					} `json:"owner"`
					Projects struct {
						Nodes []struct {
							Name   string `json:"name"`
							Status *struct {
								Name string `json:"name"`
							} `json:"status"`
						} `json:"nodes"`
					} `json:"projects"`
				} `json:"initiative"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil || parsed.Initiative == nil {
				errors.HandleError(errors.NewNotFoundError("Initiative", args[0]), "Failed to fetch initiative details")
				os.Exit(1)
			}
			init := parsed.Initiative
			if jsonOut {
				var pretty any
				_ = json.Unmarshal(data, &pretty)
				if m, ok := pretty.(map[string]any); ok {
					pretty = m["initiative"]
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(pretty)
				return
			}
			icon := ""
			if init.Icon != nil {
				icon = *init.Icon + " "
			}
			fmt.Fprintf(cmd.OutOrStdout(), "# %s%s\n\n", icon, init.Name)
			fmt.Fprintf(cmd.OutOrStdout(), "**Slug:** %s\n", init.SlugID)
			fmt.Fprintf(cmd.OutOrStdout(), "**URL:** %s\n", init.URL)
			fmt.Fprintf(cmd.OutOrStdout(), "**Status:** %s\n", init.Status)
			if init.Health != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "**Health:** %s\n", *init.Health)
			}
			if init.Owner != nil {
				who := init.Owner.DisplayName
				if who == "" {
					who = init.Owner.Name
				}
				fmt.Fprintf(cmd.OutOrStdout(), "**Owner:** %s\n", who)
			}
			if init.TargetDate != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "**Target Date:** %s\n", *init.TargetDate)
			}
			if init.Description != nil && *init.Description != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "\n## Description\n\n%s\n", *init.Description)
			}
			projects := init.Projects.Nodes
			fmt.Fprintf(cmd.OutOrStdout(), "\n## Projects (%d)\n\n", len(projects))
			if len(projects) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "*No projects linked to this initiative.*")
			} else {
				for _, p := range projects {
					status := "Unknown"
					if p.Status != nil {
						status = p.Status.Name
					}
					fmt.Fprintf(cmd.OutOrStdout(), "- **%s** (%s)\n", p.Name, status)
				}
			}
		},
	}
	cmd.Flags().BoolVarP(&jsonOut, "json", "j", false, "Output as JSON")
	return cmd
}

func newCreateCommand() *cobra.Command {
	var name, description, status string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an initiative",
		Run: func(cmd *cobra.Command, args []string) {
			if name == "" {
				errors.HandleError(
					errors.NewValidationError("Initiative name is required", errors.WithSuggestion("Use --name or -n")),
					"Failed to create initiative",
				)
				os.Exit(1)
			}
			input := map[string]any{"name": name}
			if description != "" {
				input["description"] = description
			}
			if status != "" {
				mapped := map[string]string{
					"planned": "Planned", "active": "Active", "completed": "Completed",
				}
				apiStatus, ok := mapped[strings.ToLower(status)]
				if !ok {
					errors.HandleError(
						errors.NewValidationError("invalid status", errors.WithSuggestion("Use planned, active, or completed")),
						"Failed to create initiative",
					)
					os.Exit(1)
				}
				input["status"] = apiStatus
			}
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to create initiative")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), `
mutation CreateInitiative($input: InitiativeCreateInput!) {
  initiativeCreate(input: $input) {
    success
    initiative { id slugId name url }
  }
}`, map[string]any{"input": input})
			if err != nil {
				errors.HandleError(err, "Failed to create initiative")
				os.Exit(1)
			}
			var parsed struct {
				InitiativeCreate struct {
					Success    bool `json:"success"`
					Initiative *struct {
						Name   string `json:"name"`
						SlugID string `json:"slugId"`
						URL    string `json:"url"`
					} `json:"initiative"`
				} `json:"initiativeCreate"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil || !parsed.InitiativeCreate.Success || parsed.InitiativeCreate.Initiative == nil {
				errors.HandleError(errors.NewCliError("Failed to create initiative"), "Failed to create initiative")
				os.Exit(1)
			}
			i := parsed.InitiativeCreate.Initiative
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Created initiative: %s\n", i.Name)
			fmt.Fprintf(cmd.OutOrStdout(), "  Slug: %s\n", i.SlugID)
			if i.URL != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  URL: %s\n", i.URL)
			}
		},
	}
	cmd.Flags().StringVarP(&name, "name", "n", "", "Initiative name")
	cmd.Flags().StringVarP(&description, "description", "d", "", "Description")
	cmd.Flags().StringVarP(&status, "status", "s", "", "Status (planned, active, completed)")
	return cmd
}

func newUpdateCommand() *cobra.Command {
	var name, description, status string
	cmd := &cobra.Command{
		Use:   "update <initiativeId>",
		Short: "Update an initiative",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if name == "" && description == "" && status == "" {
				errors.HandleError(
					errors.NewValidationError("At least one update option required"),
					"Failed to update initiative",
				)
				os.Exit(1)
			}
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to update initiative")
				os.Exit(1)
			}
			id, err := resolveInitiativeID(client, args[0])
			if err != nil {
				errors.HandleError(err, "Failed to update initiative")
				os.Exit(1)
			}
			input := map[string]any{}
			if name != "" {
				input["name"] = name
			}
			if description != "" {
				input["description"] = description
			}
			if status != "" {
				mapped := map[string]string{
					"planned": "Planned", "active": "Active", "completed": "Completed",
				}
				apiStatus, ok := mapped[strings.ToLower(status)]
				if !ok {
					errors.HandleError(errors.NewValidationError("invalid status"), "Failed to update initiative")
					os.Exit(1)
				}
				input["status"] = apiStatus
			}
			data, err := client.RequestRaw(context.Background(), `
mutation UpdateInitiative($id: String!, $input: InitiativeUpdateInput!) {
  initiativeUpdate(id: $id, input: $input) {
    success
    initiative { name url }
  }
}`, map[string]any{"id": id, "input": input})
			if err != nil {
				errors.HandleError(err, "Failed to update initiative")
				os.Exit(1)
			}
			var parsed struct {
				InitiativeUpdate struct {
					Success    bool `json:"success"`
					Initiative *struct {
						Name string `json:"name"`
						URL  string `json:"url"`
					} `json:"initiative"`
				} `json:"initiativeUpdate"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil || !parsed.InitiativeUpdate.Success {
				errors.HandleError(errors.NewCliError("Failed to update initiative"), "Failed to update initiative")
				os.Exit(1)
			}
			if parsed.InitiativeUpdate.Initiative != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "✓ Updated initiative: %s\n", parsed.InitiativeUpdate.Initiative.Name)
			}
		},
	}
	cmd.Flags().StringVarP(&name, "name", "n", "", "Initiative name")
	cmd.Flags().StringVarP(&description, "description", "d", "", "Description")
	cmd.Flags().StringVarP(&status, "status", "s", "", "Status")
	return cmd
}

func newDeleteCommand() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "delete <initiativeId>",
		Short: "Delete an initiative",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ok, err := prompt.Confirm(fmt.Sprintf("Are you sure you want to delete initiative %s?", args[0]), force)
			if err != nil {
				errors.HandleError(err, "Failed to delete initiative")
				os.Exit(1)
			}
			if !ok {
				fmt.Fprintln(cmd.OutOrStdout(), "Deletion canceled")
				return
			}
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to delete initiative")
				os.Exit(1)
			}
			id, err := resolveInitiativeID(client, args[0])
			if err != nil {
				errors.HandleError(err, "Failed to delete initiative")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), `
mutation DeleteInitiative($id: String!) {
  initiativeDelete(id: $id) { success }
}`, map[string]any{"id": id})
			if err != nil {
				errors.HandleError(err, "Failed to delete initiative")
				os.Exit(1)
			}
			var parsed struct {
				InitiativeDelete struct {
					Success bool `json:"success"`
				} `json:"initiativeDelete"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil || !parsed.InitiativeDelete.Success {
				errors.HandleError(errors.NewCliError("Failed to delete initiative"), "Failed to delete initiative")
				os.Exit(1)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Deleted initiative: %s\n", args[0])
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation")
	return cmd
}

func newArchiveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "archive <initiativeId>",
		Short: "Archive an initiative",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			runInitiativeArchive(cmd, args[0], true)
		},
	}
}

func newUnarchiveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "unarchive <initiativeId>",
		Short: "Unarchive an initiative",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			runInitiativeArchive(cmd, args[0], false)
		},
	}
}

func runInitiativeArchive(cmd *cobra.Command, ref string, archive bool) {
	action := "archive"
	mutation := "initiativeArchive"
	if !archive {
		action = "unarchive"
		mutation = "initiativeUnarchive"
	}
	client, err := graphql.NewClient()
	if err != nil {
		errors.HandleError(err, "Failed to "+action+" initiative")
		os.Exit(1)
	}
	id, err := resolveInitiativeID(client, ref)
	if err != nil {
		errors.HandleError(err, "Failed to "+action+" initiative")
		os.Exit(1)
	}
	q := fmt.Sprintf(`mutation($id: String!) { %s(id: $id) { success } }`, mutation)
	data, err := client.RequestRaw(context.Background(), q, map[string]any{"id": id})
	if err != nil {
		errors.HandleError(err, "Failed to "+action+" initiative")
		os.Exit(1)
	}
	var parsed map[string]struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil || !parsed[mutation].Success {
		errors.HandleError(errors.NewCliError("Failed to "+action+" initiative"), "Failed to "+action+" initiative")
		os.Exit(1)
	}
	label := "Archived"
	if !archive {
		label = "Unarchived"
	}
	fmt.Fprintf(cmd.OutOrStdout(), "✓ %s initiative: %s\n", label, ref)
}

func newAddProjectCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "add-project <initiativeId> <projectId>",
		Short: "Add a project to an initiative",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to add project")
				os.Exit(1)
			}
			initID, err := resolveInitiativeID(client, args[0])
			if err != nil {
				errors.HandleError(err, "Failed to add project")
				os.Exit(1)
			}
			projectID := args[1]
			if len(projectID) < 30 {
				data, err := client.RequestRaw(context.Background(), `
query($name: String!) {
  projects(filter: { name: { eqIgnoreCase: $name } }, first: 5) { nodes { id } }
}`, map[string]any{"name": projectID})
				if err != nil {
					errors.HandleError(err, "Failed to add project")
					os.Exit(1)
				}
				var resolved struct {
					Projects struct {
						Nodes []struct {
							ID string `json:"id"`
						} `json:"nodes"`
					} `json:"projects"`
				}
				_ = json.Unmarshal(data, &resolved)
				if len(resolved.Projects.Nodes) == 0 {
					errors.HandleError(errors.NewNotFoundError("Project", args[1]), "Failed to add project")
					os.Exit(1)
				}
				projectID = resolved.Projects.Nodes[0].ID
			}
			data, err := client.RequestRaw(context.Background(), `
mutation($input: InitiativeToProjectCreateInput!) {
  initiativeToProjectCreate(input: $input) { success }
}`, map[string]any{"input": map[string]any{"initiativeId": initID, "projectId": projectID}})
			if err != nil {
				errors.HandleError(err, "Failed to add project")
				os.Exit(1)
			}
			var parsed struct {
				InitiativeToProjectCreate struct {
					Success bool `json:"success"`
				} `json:"initiativeToProjectCreate"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil || !parsed.InitiativeToProjectCreate.Success {
				errors.HandleError(errors.NewCliError("Failed to add project"), "Failed to add project")
				os.Exit(1)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "✓ Added project to initiative")
		},
	}
}

func newRemoveProjectCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "remove-project <initiativeId> <projectId>",
		Short: "Remove a project from an initiative",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to remove project")
				os.Exit(1)
			}
			initID, err := resolveInitiativeID(client, args[0])
			if err != nil {
				errors.HandleError(err, "Failed to remove project")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), `
query($id: String!) {
  initiative(id: $id) {
    initiativeToProjects(first: 100) {
      nodes { id project { id name } }
    }
  }
}`, map[string]any{"id": initID})
			if err != nil {
				errors.HandleError(err, "Failed to remove project")
				os.Exit(1)
			}
			var parsed struct {
				Initiative *struct {
					InitiativeToProjects struct {
						Nodes []struct {
							ID      string `json:"id"`
							Project struct {
								ID   string `json:"id"`
								Name string `json:"name"`
							} `json:"project"`
						} `json:"nodes"`
					} `json:"initiativeToProjects"`
				} `json:"initiative"`
			}
			_ = json.Unmarshal(data, &parsed)
			if parsed.Initiative == nil {
				errors.HandleError(errors.NewNotFoundError("Initiative", args[0]), "Failed to remove project")
				os.Exit(1)
			}
			var linkID string
			for _, n := range parsed.Initiative.InitiativeToProjects.Nodes {
				if n.Project.ID == args[1] || strings.EqualFold(n.Project.Name, args[1]) {
					linkID = n.ID
					break
				}
			}
			if linkID == "" {
				errors.HandleError(errors.NewNotFoundError("Project link", args[1]), "Failed to remove project")
				os.Exit(1)
			}
			del, err := client.RequestRaw(context.Background(), `
mutation($id: String!) {
  initiativeToProjectDelete(id: $id) { success }
}`, map[string]any{"id": linkID})
			if err != nil {
				errors.HandleError(err, "Failed to remove project")
				os.Exit(1)
			}
			var delParsed struct {
				InitiativeToProjectDelete struct {
					Success bool `json:"success"`
				} `json:"initiativeToProjectDelete"`
			}
			if err := json.Unmarshal(del, &delParsed); err != nil || !delParsed.InitiativeToProjectDelete.Success {
				errors.HandleError(errors.NewCliError("Failed to remove project"), "Failed to remove project")
				os.Exit(1)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "✓ Removed project from initiative")
		},
	}
}

func newCommentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "comment",
		Short: "Manage initiative comments",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newInitCommentListCommand(), newInitCommentAddCommand())
	return cmd
}

func newInitCommentListCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list <initiativeId>",
		Short: "List initiative comments",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to list comments")
				os.Exit(1)
			}
			id, err := resolveInitiativeID(client, args[0])
			if err != nil {
				errors.HandleError(err, "Failed to list comments")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), `
query($id: String!) {
  initiative(id: $id) {
    comments(first: 50) {
      nodes { id body createdAt user { name displayName } }
      pageInfo { hasNextPage endCursor }
    }
  }
}`, map[string]any{"id": id})
			if err != nil {
				errors.HandleError(err, "Failed to list comments")
				os.Exit(1)
			}
			var parsed struct {
				Initiative *struct {
					Comments struct {
						Nodes    []json.RawMessage `json:"nodes"`
						PageInfo json.RawMessage   `json:"pageInfo"`
					} `json:"comments"`
				} `json:"initiative"`
			}
			_ = json.Unmarshal(data, &parsed)
			if parsed.Initiative == nil {
				errors.HandleError(errors.NewNotFoundError("Initiative", args[0]), "Failed to list comments")
				os.Exit(1)
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(map[string]any{
					"nodes":    parsed.Initiative.Comments.Nodes,
					"pageInfo": parsed.Initiative.Comments.PageInfo,
				})
				return
			}
			for _, raw := range parsed.Initiative.Comments.Nodes {
				var c struct {
					Body string `json:"body"`
					User *struct {
						DisplayName string `json:"displayName"`
						Name        string `json:"name"`
					} `json:"user"`
				}
				_ = json.Unmarshal(raw, &c)
				who := ""
				if c.User != nil {
					who = c.User.DisplayName
					if who == "" {
						who = c.User.Name
					}
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", who, c.Body)
			}
		},
	}
	cmd.Flags().BoolVarP(&jsonOut, "json", "j", false, "Output as JSON")
	return cmd
}

func newInitCommentAddCommand() *cobra.Command {
	var body string
	cmd := &cobra.Command{
		Use:   "add <initiativeId>",
		Short: "Add a comment to an initiative",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if strings.TrimSpace(body) == "" {
				errors.HandleError(
					errors.NewValidationError("comment body is required", errors.WithSuggestion("Pass --body")),
					"Failed to add comment",
				)
				os.Exit(1)
			}
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to add comment")
				os.Exit(1)
			}
			id, err := resolveInitiativeID(client, args[0])
			if err != nil {
				errors.HandleError(err, "Failed to add comment")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), `
mutation($input: CommentCreateInput!) {
  commentCreate(input: $input) { success }
}`, map[string]any{"input": map[string]any{"initiativeId": id, "body": body}})
			if err != nil {
				errors.HandleError(err, "Failed to add comment")
				os.Exit(1)
			}
			var parsed struct {
				CommentCreate struct {
					Success bool `json:"success"`
				} `json:"commentCreate"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil || !parsed.CommentCreate.Success {
				errors.HandleError(errors.NewCliError("Failed to add comment"), "Failed to add comment")
				os.Exit(1)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "✓ Comment added")
		},
	}
	cmd.Flags().StringVarP(&body, "body", "b", "", "Comment body")
	return cmd
}

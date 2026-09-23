package cmdproject

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/kongken/linear-cli/internal/config"
	"github.com/kongken/linear-cli/internal/errors"
	"github.com/kongken/linear-cli/internal/graphql"
	"github.com/kongken/linear-cli/internal/ids"
	"github.com/kongken/linear-cli/internal/prompt"
	"github.com/kongken/linear-cli/internal/templates"
	"github.com/spf13/cobra"
)

var dateYYYYMMDD = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

const getProjectsQuery = `
query GetProjects($filter: ProjectFilter, $first: Int, $after: String) {
  projects(filter: $filter, first: $first, after: $after) {
    nodes {
      id
      name
      description
      slugId
      url
      status { id name color type }
      lead { name displayName }
      teams { nodes { key } }
    }
    pageInfo { hasNextPage endCursor }
  }
}
`

// New returns the project command group.
func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage Linear projects",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		newListCommand(),
		newViewCommand(),
		newDescriptionCommand(),
		newCreateCommand(),
		newUpdateCommand(),
		newDeleteCommand(),
		newCommentCommand(),
	)
	return cmd
}

func newListCommand() *cobra.Command {
	var (
		team    string
		jsonOut bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List projects",
		Run: func(cmd *cobra.Command, args []string) {
			teamKey := strings.ToUpper(team)
			if teamKey == "" {
				if t, ok := config.GetOption("team_id"); ok {
					teamKey = strings.ToUpper(t)
				}
			}
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to list projects")
				os.Exit(1)
			}
			vars := map[string]any{"first": 50}
			if teamKey != "" {
				vars["filter"] = map[string]any{
					"accessibleTeams": map[string]any{
						"some": map[string]any{"key": map[string]any{"eq": teamKey}},
					},
				}
			}
			data, err := client.RequestRaw(context.Background(), getProjectsQuery, vars)
			if err != nil {
				errors.HandleError(err, "Failed to list projects")
				os.Exit(1)
			}
			var parsed struct {
				Projects struct {
					Nodes    []json.RawMessage `json:"nodes"`
					PageInfo json.RawMessage   `json:"pageInfo"`
				} `json:"projects"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil {
				errors.HandleError(err, "Failed to list projects")
				os.Exit(1)
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(map[string]any{
					"nodes":    parsed.Projects.Nodes,
					"pageInfo": parsed.Projects.PageInfo,
				})
				return
			}
			for _, raw := range parsed.Projects.Nodes {
				var p struct {
					Name string `json:"name"`
					URL  string `json:"url"`
				}
				_ = json.Unmarshal(raw, &p)
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", p.Name, p.URL)
			}
		},
	}
	cmd.Flags().StringVar(&team, "team", "", "Filter by team key")
	cmd.Flags().BoolVarP(&jsonOut, "json", "j", false, "Output as JSON")
	return cmd
}

const getProjectQuery = `
query GetProject($id: String!) {
  project(id: $id) {
    id name description slugId url
    status { name type }
    lead { name displayName }
    teams { nodes { key } }
  }
}
`

func resolveProjectRef(client *graphql.Client, ref string) (string, error) {
	if ids.IsUUID(ref) {
		return ref, nil
	}
	data, err := client.RequestRaw(context.Background(), `
query ResolveProjectRef($name: String!) {
  projects(filter: { name: { eqIgnoreCase: $name } }, first: 5) {
    nodes { id name slugId }
  }
}`, map[string]any{"name": ref})
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

func newViewCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "view <projectId>",
		Short: "View a project",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to view project")
				os.Exit(1)
			}
			id, err := resolveProjectRef(client, args[0])
			if err != nil {
				errors.HandleError(err, "Failed to view project")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), getProjectQuery, map[string]any{"id": id})
			if err != nil {
				errors.HandleError(err, "Failed to view project")
				os.Exit(1)
			}
			var parsed struct {
				Project json.RawMessage `json:"project"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil || string(parsed.Project) == "null" {
				errors.HandleError(errors.NewNotFoundError("Project", args[0]), "Failed to view project")
				os.Exit(1)
			}
			if jsonOut {
				var pretty any
				_ = json.Unmarshal(parsed.Project, &pretty)
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(pretty)
				return
			}
			var p struct {
				Name        string  `json:"name"`
				URL         string  `json:"url"`
				Description *string `json:"description"`
				Status      *struct {
					Name string `json:"name"`
				} `json:"status"`
			}
			_ = json.Unmarshal(parsed.Project, &p)
			fmt.Fprintf(cmd.OutOrStdout(), "# %s\n", p.Name)
			if p.Status != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Status: %s\n", p.Status.Name)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "URL: %s\n", p.URL)
			if p.Description != nil && *p.Description != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "\n%s\n", *p.Description)
			}
		},
	}
	cmd.Flags().BoolVarP(&jsonOut, "json", "j", false, "Output as JSON")
	return cmd
}

func newDescriptionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "description <projectId>",
		Short: "Print project description",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to get project description")
				os.Exit(1)
			}
			id, err := resolveProjectRef(client, args[0])
			if err != nil {
				errors.HandleError(err, "Failed to get project description")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), getProjectQuery, map[string]any{"id": id})
			if err != nil {
				errors.HandleError(err, "Failed to get project description")
				os.Exit(1)
			}
			var parsed struct {
				Project *struct {
					Description *string `json:"description"`
				} `json:"project"`
			}
			_ = json.Unmarshal(data, &parsed)
			if parsed.Project == nil {
				errors.HandleError(errors.NewNotFoundError("Project", args[0]), "Failed to get project description")
				os.Exit(1)
			}
			if parsed.Project.Description != nil {
				fmt.Fprintln(cmd.OutOrStdout(), *parsed.Project.Description)
			}
		},
	}
}

const createProjectMutation = `
mutation CreateProject($input: ProjectCreateInput!) {
  projectCreate(input: $input) {
    success
    project { id slugId name url }
  }
}
`

const updateProjectMutation = `
mutation UpdateProject($id: String!, $input: ProjectUpdateInput!) {
  projectUpdate(id: $id, input: $input) {
    success
    project { id slugId name url }
  }
}
`

const deleteProjectMutation = `
mutation DeleteProject($id: String!) {
  projectDelete(id: $id) {
    success
    entity { id name }
  }
}
`

func newCreateCommand() *cobra.Command {
	var (
		name, description, descriptionFile string
		content, contentFile               string
		lead, status, priority             string
		startDate, targetDate              string
		icon, color, initiative, template  string
		teams, labels, members             []string
		jsonOut, noInteractive             bool
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a project",
		Run: func(cmd *cobra.Command, args []string) {
			nameVal := name
			descVal := description
			leadVal := lead
			statusVal := status
			teamKeys := make([]string, 0, len(teams))
			for _, t := range teams {
				if strings.TrimSpace(t) != "" {
					teamKeys = append(teamKeys, strings.ToUpper(strings.TrimSpace(t)))
				}
			}

			if description != "" && descriptionFile != "" {
				errors.HandleError(
					errors.NewValidationError("Cannot specify both --description and --description-file"),
					"Failed to create project",
				)
				os.Exit(1)
			}
			if content != "" && contentFile != "" {
				errors.HandleError(
					errors.NewValidationError("Cannot specify both --content and --content-file"),
					"Failed to create project",
				)
				os.Exit(1)
			}

			interactive := !noInteractive && prompt.IsInteractive() && nameVal == "" && len(teamKeys) == 0
			if interactive {
				seed := ""
				if len(teamKeys) > 0 {
					seed = teamKeys[0]
				}
				wiz, err := runInteractiveProjectCreate(seed)
				if err != nil {
					errors.HandleError(err, "Failed to create project")
					os.Exit(1)
				}
				nameVal = wiz.Name
				if wiz.Description != "" {
					descVal = wiz.Description
				}
				teamKeys = []string{wiz.TeamKey}
				if wiz.Status != "" {
					statusVal = wiz.Status
				}
				if wiz.Lead != "" {
					leadVal = wiz.Lead
				}
			}

			if descriptionFile != "" {
				b, err := os.ReadFile(descriptionFile)
				if err != nil {
					errors.HandleError(err, "Failed to create project")
					os.Exit(1)
				}
				descVal = string(b)
			}
			contentVal := content
			if contentFile != "" {
				b, err := os.ReadFile(contentFile)
				if err != nil {
					errors.HandleError(err, "Failed to create project")
					os.Exit(1)
				}
				contentVal = string(b)
			}

			if nameVal == "" {
				errors.HandleError(
					errors.NewValidationError("Project name is required", errors.WithSuggestion("Use --name or -n")),
					"Failed to create project",
				)
				os.Exit(1)
			}
			if len(teamKeys) == 0 {
				if t, ok := config.GetOption("team_id"); ok && t != "" {
					teamKeys = append(teamKeys, strings.ToUpper(t))
				}
			}
			if len(teamKeys) == 0 {
				errors.HandleError(
					errors.NewValidationError("At least one team is required", errors.WithSuggestion("Use --team or -t")),
					"Failed to create project",
				)
				os.Exit(1)
			}
			if startDate != "" && !dateYYYYMMDD.MatchString(startDate) {
				errors.HandleError(errors.NewValidationError("Start date must be in YYYY-MM-DD format"), "Failed to create project")
				os.Exit(1)
			}
			if targetDate != "" && !dateYYYYMMDD.MatchString(targetDate) {
				errors.HandleError(errors.NewValidationError("Target date must be in YYYY-MM-DD format"), "Failed to create project")
				os.Exit(1)
			}

			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to create project")
				os.Exit(1)
			}
			teamIDs := make([]string, 0, len(teamKeys))
			for _, key := range teamKeys {
				tid, err := resolveTeamID(client, key)
				if err != nil {
					errors.HandleError(err, "Failed to create project")
					os.Exit(1)
				}
				teamIDs = append(teamIDs, tid)
			}
			input := map[string]any{"name": nameVal, "teamIds": teamIDs}
			if descVal != "" {
				input["description"] = descVal
			}
			if contentVal != "" {
				input["content"] = contentVal
			}
			if statusVal != "" {
				statusID, err := resolveProjectStatusID(client, statusVal)
				if err != nil {
					errors.HandleError(err, "Failed to create project")
					os.Exit(1)
				}
				input["statusId"] = statusID
			}
			if leadVal != "" {
				leadID, err := lookupUserID(client, leadVal)
				if err != nil {
					errors.HandleError(err, "Failed to create project")
					os.Exit(1)
				}
				input["leadId"] = leadID
			}
			if priority != "" {
				p, err := parseProjectPriority(priority)
				if err != nil {
					errors.HandleError(err, "Failed to create project")
					os.Exit(1)
				}
				input["priority"] = p
			}
			if startDate != "" {
				input["startDate"] = startDate
			}
			if targetDate != "" {
				input["targetDate"] = targetDate
			}
			if icon != "" {
				input["icon"] = icon
			}
			if color != "" {
				input["color"] = color
			}
			if len(labels) > 0 {
				labelIDs, err := resolveProjectLabelIDs(client, labels)
				if err != nil {
					errors.HandleError(err, "Failed to create project")
					os.Exit(1)
				}
				input["labelIds"] = labelIDs
			}
			if len(members) > 0 {
				memberIDs := make([]string, 0, len(members))
				for _, m := range members {
					id, err := lookupUserID(client, m)
					if err != nil {
						errors.HandleError(err, "Failed to create project")
						os.Exit(1)
					}
					memberIDs = append(memberIDs, id)
				}
				input["memberIds"] = memberIDs
			}
			if template != "" {
				tmpl, err := templates.Resolve(context.Background(), template, &templates.Scope{
					Type:    "project",
					TeamIDs: teamIDs,
				})
				if err != nil {
					errors.HandleError(err, "Failed to create project")
					os.Exit(1)
				}
				input["templateId"] = tmpl.ID
			}

			data, err := client.RequestRaw(context.Background(), createProjectMutation, map[string]any{"input": input})
			if err != nil {
				errors.HandleError(err, "Failed to create project")
				os.Exit(1)
			}
			var parsed struct {
				ProjectCreate struct {
					Success bool `json:"success"`
					Project *struct {
						ID     string `json:"id"`
						Name   string `json:"name"`
						SlugID string `json:"slugId"`
						URL    string `json:"url"`
					} `json:"project"`
				} `json:"projectCreate"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil || !parsed.ProjectCreate.Success || parsed.ProjectCreate.Project == nil {
				errors.HandleError(errors.NewCliError("Failed to create project"), "Failed to create project")
				os.Exit(1)
			}
			p := parsed.ProjectCreate.Project
			if initiative != "" {
				initIDs, err := resolveInitiativeIDs(client, []string{initiative})
				if err != nil {
					errors.HandleError(err, "Failed to create project")
					os.Exit(1)
				}
				_, err = client.RequestRaw(context.Background(), `
mutation AddProjectToInitiativeForCreate($input: InitiativeToProjectCreateInput!) {
  initiativeToProjectCreate(input: $input) { success }
}`, map[string]any{"input": map[string]any{"initiativeId": initIDs[0], "projectId": p.ID}})
				if err != nil {
					errors.HandleError(err, "Failed to link project to initiative")
					os.Exit(1)
				}
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(parsed.ProjectCreate)
				return
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Created project: %s\n", p.Name)
			fmt.Fprintf(cmd.OutOrStdout(), "  Slug: %s\n", p.SlugID)
			if p.URL != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  URL: %s\n", p.URL)
			}
		},
	}
	cmd.Flags().StringVarP(&name, "name", "n", "", "Project name")
	cmd.Flags().StringVarP(&description, "description", "d", "", "Project description")
	cmd.Flags().StringVar(&descriptionFile, "description-file", "", "Read description from file")
	cmd.Flags().StringVar(&content, "content", "", "Project overview markdown")
	cmd.Flags().StringVar(&contentFile, "content-file", "", "Read overview markdown from file")
	cmd.Flags().StringArrayVarP(&teams, "team", "t", nil, "Team key (repeatable)")
	cmd.Flags().StringVarP(&lead, "lead", "l", "", "Project lead (@me, username, or email)")
	cmd.Flags().StringVarP(&status, "status", "s", "", "Status type (planned, started, paused, completed, canceled, backlog)")
	cmd.Flags().StringVar(&startDate, "start-date", "", "Start date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&targetDate, "target-date", "", "Target date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&priority, "priority", "", "Priority (none, urgent, high, medium, low)")
	cmd.Flags().StringArrayVar(&labels, "label", nil, "Project label name (repeatable)")
	cmd.Flags().StringArrayVar(&members, "member", nil, "Project member (repeatable)")
	cmd.Flags().StringVar(&icon, "icon", "", "Project icon")
	cmd.Flags().StringVar(&color, "color", "", "Project color as HEX")
	cmd.Flags().StringVar(&initiative, "initiative", "", "Add to initiative (ID, slug, or name)")
	cmd.Flags().StringVar(&template, "template", "", "Project template by name or ID")
	cmd.Flags().BoolVarP(&jsonOut, "json", "j", false, "Output as JSON")
	cmd.Flags().BoolVar(&noInteractive, "no-interactive", false, "Disable interactive prompts")
	return cmd
}

func newUpdateCommand() *cobra.Command {
	var (
		name, description, status, lead string
		startDate, targetDate           string
		clearLead, clearStartDate       bool
		clearTargetDate                 bool
		teamsFlag, addTeam, removeTeam  []string
		labelsFlag, addLabel, removeLabel []string
		initiatives, addInitiative, removeInitiative []string
	)
	cmd := &cobra.Command{
		Use:   "update <projectId>",
		Short: "Update a project",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			hasUpdate := name != "" || description != "" || status != "" || lead != "" ||
				startDate != "" || targetDate != "" || clearLead || clearStartDate || clearTargetDate ||
				len(teamsFlag) > 0 || len(addTeam) > 0 || len(removeTeam) > 0 ||
				len(labelsFlag) > 0 || len(addLabel) > 0 || len(removeLabel) > 0 ||
				len(initiatives) > 0 || len(addInitiative) > 0 || len(removeInitiative) > 0
			if !hasUpdate {
				errors.HandleError(
					errors.NewValidationError(
						"At least one update option must be provided",
						errors.WithSuggestion("Use --name, --description, --status, --lead, --team, --label, or --initiative flags"),
					),
					"Failed to update project",
				)
				os.Exit(1)
			}
			if clearLead && lead != "" {
				errors.HandleError(
					errors.NewValidationError("Cannot specify both --lead and --clear-lead"),
					"Failed to update project",
				)
				os.Exit(1)
			}
			if len(teamsFlag) > 0 && (len(addTeam) > 0 || len(removeTeam) > 0) {
				errors.HandleError(
					errors.NewValidationError("Cannot combine --team with --add-team or --remove-team"),
					"Failed to update project",
				)
				os.Exit(1)
			}
			if len(labelsFlag) > 0 && (len(addLabel) > 0 || len(removeLabel) > 0) {
				errors.HandleError(
					errors.NewValidationError("Cannot combine --label with --add-label or --remove-label"),
					"Failed to update project",
				)
				os.Exit(1)
			}
			if len(initiatives) > 0 && (len(addInitiative) > 0 || len(removeInitiative) > 0) {
				errors.HandleError(
					errors.NewValidationError("Cannot combine --initiative with --add-initiative or --remove-initiative"),
					"Failed to update project",
				)
				os.Exit(1)
			}
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to update project")
				os.Exit(1)
			}
			id, err := resolveProjectRef(client, args[0])
			if err != nil {
				errors.HandleError(err, "Failed to update project")
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
				statusID, err := resolveProjectStatusID(client, status)
				if err != nil {
					errors.HandleError(err, "Failed to update project")
					os.Exit(1)
				}
				input["statusId"] = statusID
			}
			if clearLead {
				input["leadId"] = nil
			} else if lead != "" {
				leadID, err := lookupUserID(client, lead)
				if err != nil {
					errors.HandleError(err, "Failed to update project")
					os.Exit(1)
				}
				input["leadId"] = leadID
			}
			if clearStartDate {
				input["startDate"] = nil
			} else if startDate != "" {
				input["startDate"] = startDate
			}
			if clearTargetDate {
				input["targetDate"] = nil
			} else if targetDate != "" {
				input["targetDate"] = targetDate
			}
			if len(teamsFlag) > 0 {
				ids := make([]string, 0, len(teamsFlag))
				for _, t := range teamsFlag {
					tid, err := resolveTeamID(client, t)
					if err != nil {
						errors.HandleError(err, "Failed to update project")
						os.Exit(1)
					}
					ids = append(ids, tid)
				}
				input["teamIds"] = ids
			} else if len(addTeam) > 0 || len(removeTeam) > 0 {
				current, err := fetchProjectTeamIDs(client, id)
				if err != nil {
					errors.HandleError(err, "Failed to update project")
					os.Exit(1)
				}
				removeSet := map[string]bool{}
				for _, t := range removeTeam {
					tid, err := resolveTeamID(client, t)
					if err != nil {
						errors.HandleError(err, "Failed to update project")
						os.Exit(1)
					}
					removeSet[tid] = true
				}
				next := make([]string, 0, len(current)+len(addTeam))
				for _, tid := range current {
					if !removeSet[tid] {
						next = append(next, tid)
					}
				}
				for _, t := range addTeam {
					tid, err := resolveTeamID(client, t)
					if err != nil {
						errors.HandleError(err, "Failed to update project")
						os.Exit(1)
					}
					found := false
					for _, existing := range next {
						if existing == tid {
							found = true
							break
						}
					}
					if !found {
						next = append(next, tid)
					}
				}
				if len(next) == 0 {
					errors.HandleError(
						errors.NewValidationError("Removing these teams would leave the project with no teams; Linear requires at least one"),
						"Failed to update project",
					)
					os.Exit(1)
				}
				input["teamIds"] = next
			}
			if len(labelsFlag) > 0 {
				ids, err := resolveProjectLabelIDs(client, labelsFlag)
				if err != nil {
					errors.HandleError(err, "Failed to update project")
					os.Exit(1)
				}
				input["labelIds"] = ids
			} else if len(addLabel) > 0 || len(removeLabel) > 0 {
				current, err := fetchProjectLabelIDs(client, id)
				if err != nil {
					errors.HandleError(err, "Failed to update project")
					os.Exit(1)
				}
				removeIDs, err := resolveProjectLabelIDs(client, removeLabel)
				if err != nil {
					errors.HandleError(err, "Failed to update project")
					os.Exit(1)
				}
				removeSet := map[string]bool{}
				for _, rid := range removeIDs {
					removeSet[rid] = true
				}
				next := make([]string, 0, len(current)+len(addLabel))
				for _, lid := range current {
					if !removeSet[lid] {
						next = append(next, lid)
					}
				}
				addIDs, err := resolveProjectLabelIDs(client, addLabel)
				if err != nil {
					errors.HandleError(err, "Failed to update project")
					os.Exit(1)
				}
				for _, lid := range addIDs {
					found := false
					for _, existing := range next {
						if existing == lid {
							found = true
							break
						}
					}
					if !found {
						next = append(next, lid)
					}
				}
				input["labelIds"] = next
			}
			var projectName, projectURL string
			if len(input) > 0 {
				data, err := client.RequestRaw(context.Background(), updateProjectMutation, map[string]any{
					"id": id, "input": input,
				})
				if err != nil {
					errors.HandleError(err, "Failed to update project")
					os.Exit(1)
				}
				var parsed struct {
					ProjectUpdate struct {
						Success bool `json:"success"`
						Project *struct {
							Name string `json:"name"`
							URL  string `json:"url"`
						} `json:"project"`
					} `json:"projectUpdate"`
				}
				if err := json.Unmarshal(data, &parsed); err != nil || !parsed.ProjectUpdate.Success {
					errors.HandleError(errors.NewCliError("Failed to update project"), "Failed to update project")
					os.Exit(1)
				}
				if parsed.ProjectUpdate.Project != nil {
					projectName = parsed.ProjectUpdate.Project.Name
					projectURL = parsed.ProjectUpdate.Project.URL
				}
			}
			if len(initiatives) > 0 || len(addInitiative) > 0 || len(removeInitiative) > 0 {
				if err := applyProjectInitiativeChanges(client, id, initiatives, addInitiative, removeInitiative); err != nil {
					errors.HandleError(err, "Failed to update project")
					os.Exit(1)
				}
				if projectName == "" {
					projectName = args[0]
				}
			}
			if projectName != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "✓ Updated project: %s\n", projectName)
				if projectURL != "" {
					fmt.Fprintln(cmd.OutOrStdout(), projectURL)
				}
			}
		},
	}
	cmd.Flags().StringVarP(&name, "name", "n", "", "Project name")
	cmd.Flags().StringVarP(&description, "description", "d", "", "Project description")
	cmd.Flags().StringVarP(&status, "status", "s", "", "Status type")
	cmd.Flags().StringVarP(&lead, "lead", "l", "", "Project lead")
	cmd.Flags().BoolVar(&clearLead, "clear-lead", false, "Remove project lead")
	cmd.Flags().StringVar(&startDate, "start-date", "", "Start date (YYYY-MM-DD)")
	cmd.Flags().BoolVar(&clearStartDate, "clear-start-date", false, "Clear start date")
	cmd.Flags().StringVar(&targetDate, "target-date", "", "Target date (YYYY-MM-DD)")
	cmd.Flags().BoolVar(&clearTargetDate, "clear-target-date", false, "Clear target date")
	cmd.Flags().StringArrayVarP(&teamsFlag, "team", "t", nil, "Replace team set (repeatable)")
	cmd.Flags().StringArrayVar(&addTeam, "add-team", nil, "Add team (repeatable)")
	cmd.Flags().StringArrayVar(&removeTeam, "remove-team", nil, "Remove team (repeatable)")
	cmd.Flags().StringArrayVar(&labelsFlag, "label", nil, "Replace label set (repeatable)")
	cmd.Flags().StringArrayVar(&addLabel, "add-label", nil, "Add label (repeatable)")
	cmd.Flags().StringArrayVar(&removeLabel, "remove-label", nil, "Remove label (repeatable)")
	cmd.Flags().StringArrayVar(&initiatives, "initiative", nil, "Replace initiative set (repeatable)")
	cmd.Flags().StringArrayVar(&addInitiative, "add-initiative", nil, "Add initiative (repeatable)")
	cmd.Flags().StringArrayVar(&removeInitiative, "remove-initiative", nil, "Remove initiative (repeatable)")
	return cmd
}

func applyProjectInitiativeChanges(client *graphql.Client, projectID string, replace, add, remove []string) error {
	links, err := fetchProjectInitiativeLinks(client, projectID)
	if err != nil {
		return err
	}
	currentIDs := make([]string, 0, len(links))
	linkByInit := map[string]string{}
	for _, l := range links {
		currentIDs = append(currentIDs, l.InitiativeID)
		linkByInit[l.InitiativeID] = l.LinkID
	}
	var desired []string
	if len(replace) > 0 {
		desired, err = resolveInitiativeIDs(client, replace)
		if err != nil {
			return err
		}
	} else {
		removeIDs, err := resolveInitiativeIDs(client, remove)
		if err != nil {
			return err
		}
		removeSet := map[string]bool{}
		for _, id := range removeIDs {
			removeSet[id] = true
		}
		desired = make([]string, 0, len(currentIDs)+len(add))
		for _, id := range currentIDs {
			if !removeSet[id] {
				desired = append(desired, id)
			}
		}
		addIDs, err := resolveInitiativeIDs(client, add)
		if err != nil {
			return err
		}
		for _, id := range addIDs {
			found := false
			for _, existing := range desired {
				if existing == id {
					found = true
					break
				}
			}
			if !found {
				desired = append(desired, id)
			}
		}
	}
	desiredSet := map[string]bool{}
	for _, id := range desired {
		desiredSet[id] = true
	}
	currentSet := map[string]bool{}
	for _, id := range currentIDs {
		currentSet[id] = true
	}
	// Deletes first.
	for _, id := range currentIDs {
		if !desiredSet[id] {
			linkID := linkByInit[id]
			data, err := client.RequestRaw(context.Background(), `
mutation($id: String!) { initiativeToProjectDelete(id: $id) { success } }`, map[string]any{"id": linkID})
			if err != nil {
				return err
			}
			var parsed struct {
				InitiativeToProjectDelete struct {
					Success bool `json:"success"`
				} `json:"initiativeToProjectDelete"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil || !parsed.InitiativeToProjectDelete.Success {
				return errors.NewCliError("Failed to remove initiative from project")
			}
		}
	}
	for _, id := range desired {
		if !currentSet[id] {
			data, err := client.RequestRaw(context.Background(), `
mutation($input: InitiativeToProjectCreateInput!) {
  initiativeToProjectCreate(input: $input) { success }
}`, map[string]any{"input": map[string]any{"initiativeId": id, "projectId": projectID}})
			if err != nil {
				return err
			}
			var parsed struct {
				InitiativeToProjectCreate struct {
					Success bool `json:"success"`
				} `json:"initiativeToProjectCreate"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil || !parsed.InitiativeToProjectCreate.Success {
				return errors.NewCliError("Failed to add initiative to project")
			}
		}
	}
	return nil
}

type initiativeLink struct {
	LinkID       string
	InitiativeID string
}

func fetchProjectInitiativeLinks(client *graphql.Client, projectID string) ([]initiativeLink, error) {
	var after *string
	var links []initiativeLink
	for {
		vars := map[string]any{"id": projectID}
		if after != nil {
			vars["after"] = *after
		}
		data, err := client.RequestRaw(context.Background(), `
query($id: String!, $after: String) {
  project(id: $id) {
    initiativeToProjects(first: 250, after: $after) {
      nodes { id initiative { id } }
      pageInfo { hasNextPage endCursor }
    }
  }
}`, vars)
		if err != nil {
			return nil, err
		}
		var parsed struct {
			Project *struct {
				InitiativeToProjects struct {
					Nodes []struct {
						ID         string `json:"id"`
						Initiative struct {
							ID string `json:"id"`
						} `json:"initiative"`
					} `json:"nodes"`
					PageInfo struct {
						HasNextPage bool    `json:"hasNextPage"`
						EndCursor   *string `json:"endCursor"`
					} `json:"pageInfo"`
				} `json:"initiativeToProjects"`
			} `json:"project"`
		}
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, err
		}
		if parsed.Project == nil {
			return nil, errors.NewNotFoundError("Project", projectID)
		}
		for _, n := range parsed.Project.InitiativeToProjects.Nodes {
			links = append(links, initiativeLink{LinkID: n.ID, InitiativeID: n.Initiative.ID})
		}
		if !parsed.Project.InitiativeToProjects.PageInfo.HasNextPage {
			break
		}
		after = parsed.Project.InitiativeToProjects.PageInfo.EndCursor
		if after == nil {
			break
		}
	}
	return links, nil
}

func resolveInitiativeIDs(client *graphql.Client, refs []string) ([]string, error) {
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ids.IsUUID(ref) {
			out = append(out, ref)
			continue
		}
		found := ""
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
				found = parsed.Initiatives.Nodes[0].ID
				break
			}
		}
		if found == "" {
			return nil, errors.NewNotFoundError("Initiative", ref)
		}
		out = append(out, found)
	}
	return out, nil
}

func fetchProjectTeamIDs(client *graphql.Client, projectID string) ([]string, error) {
	var after *string
	var ids []string
	for {
		vars := map[string]any{"id": projectID}
		if after != nil {
			vars["after"] = *after
		}
		data, err := client.RequestRaw(context.Background(), `
query GetProjectTeamsForUpdate($id: String!, $after: String) {
  project(id: $id) {
    teams(first: 250, after: $after) {
      nodes { id }
      pageInfo { hasNextPage endCursor }
    }
  }
}`, vars)
		if err != nil {
			return nil, err
		}
		var parsed struct {
			Project *struct {
				Teams struct {
					Nodes []struct {
						ID string `json:"id"`
					} `json:"nodes"`
					PageInfo struct {
						HasNextPage bool    `json:"hasNextPage"`
						EndCursor   *string `json:"endCursor"`
					} `json:"pageInfo"`
				} `json:"teams"`
			} `json:"project"`
		}
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, err
		}
		if parsed.Project == nil {
			return nil, errors.NewNotFoundError("Project", projectID)
		}
		for _, n := range parsed.Project.Teams.Nodes {
			ids = append(ids, n.ID)
		}
		if !parsed.Project.Teams.PageInfo.HasNextPage {
			break
		}
		after = parsed.Project.Teams.PageInfo.EndCursor
		if after == nil {
			break
		}
	}
	return ids, nil
}

func resolveProjectLabelIDs(client *graphql.Client, names []string) ([]string, error) {
	ids := make([]string, 0, len(names))
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			return nil, errors.NewValidationError("Project label cannot be empty")
		}
		data, err := client.RequestRaw(context.Background(), `
query($name: String!) {
  projectLabels(filter: { name: { eqIgnoreCase: $name } }) {
    nodes { id name }
  }
}`, map[string]any{"name": name})
		if err != nil {
			return nil, err
		}
		var parsed struct {
			ProjectLabels struct {
				Nodes []struct {
					ID string `json:"id"`
				} `json:"nodes"`
			} `json:"projectLabels"`
		}
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, err
		}
		if len(parsed.ProjectLabels.Nodes) == 0 {
			return nil, errors.NewNotFoundError("Project label", name)
		}
		ids = append(ids, parsed.ProjectLabels.Nodes[0].ID)
	}
	return ids, nil
}

func fetchProjectLabelIDs(client *graphql.Client, projectID string) ([]string, error) {
	var after *string
	var ids []string
	for {
		vars := map[string]any{"id": projectID}
		if after != nil {
			vars["after"] = *after
		}
		data, err := client.RequestRaw(context.Background(), `
query GetProjectLabelsForUpdate($id: String!, $after: String) {
  project(id: $id) {
    labels(first: 250, after: $after) {
      nodes { id }
      pageInfo { hasNextPage endCursor }
    }
  }
}`, vars)
		if err != nil {
			return nil, err
		}
		var parsed struct {
			Project *struct {
				Labels struct {
					Nodes []struct {
						ID string `json:"id"`
					} `json:"nodes"`
					PageInfo struct {
						HasNextPage bool    `json:"hasNextPage"`
						EndCursor   *string `json:"endCursor"`
					} `json:"pageInfo"`
				} `json:"labels"`
			} `json:"project"`
		}
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, err
		}
		if parsed.Project == nil {
			return nil, errors.NewNotFoundError("Project", projectID)
		}
		for _, n := range parsed.Project.Labels.Nodes {
			ids = append(ids, n.ID)
		}
		if !parsed.Project.Labels.PageInfo.HasNextPage {
			break
		}
		after = parsed.Project.Labels.PageInfo.EndCursor
		if after == nil {
			break
		}
	}
	return ids, nil
}

func newDeleteCommand() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "delete <projectId>",
		Short: "Delete (trash) a project",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ok, err := prompt.Confirm(fmt.Sprintf("Are you sure you want to delete project %s?", args[0]), force)
			if err != nil {
				errors.HandleError(err, "Failed to delete project")
				os.Exit(1)
			}
			if !ok {
				fmt.Fprintln(cmd.OutOrStdout(), "Deletion canceled")
				return
			}
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to delete project")
				os.Exit(1)
			}
			id, err := resolveProjectRef(client, args[0])
			if err != nil {
				errors.HandleError(err, "Failed to delete project")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), deleteProjectMutation, map[string]any{"id": id})
			if err != nil {
				errors.HandleError(err, "Failed to delete project")
				os.Exit(1)
			}
			var parsed struct {
				ProjectDelete struct {
					Success bool `json:"success"`
					Entity  *struct {
						Name string `json:"name"`
					} `json:"entity"`
				} `json:"projectDelete"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil || !parsed.ProjectDelete.Success {
				errors.HandleError(errors.NewCliError("Failed to delete project"), "Failed to delete project")
				os.Exit(1)
			}
			display := args[0]
			if parsed.ProjectDelete.Entity != nil && parsed.ProjectDelete.Entity.Name != "" {
				display = parsed.ProjectDelete.Entity.Name
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Deleted project: %s\n", display)
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")
	return cmd
}

func newCommentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "comment",
		Short: "Manage project comments",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newCommentListCommand(), newCommentAddCommand())
	return cmd
}

func newCommentListCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list <projectId>",
		Short: "List project comments",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to list project comments")
				os.Exit(1)
			}
			id, err := resolveProjectRef(client, args[0])
			if err != nil {
				errors.HandleError(err, "Failed to list project comments")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), `
query ProjectComments($id: String!) {
  project(id: $id) {
    comments(first: 50) {
      nodes { id body createdAt user { name displayName } }
      pageInfo { hasNextPage endCursor }
    }
  }
}`, map[string]any{"id": id})
			if err != nil {
				errors.HandleError(err, "Failed to list project comments")
				os.Exit(1)
			}
			var parsed struct {
				Project *struct {
					Comments struct {
						Nodes    []json.RawMessage `json:"nodes"`
						PageInfo json.RawMessage   `json:"pageInfo"`
					} `json:"comments"`
				} `json:"project"`
			}
			_ = json.Unmarshal(data, &parsed)
			if parsed.Project == nil {
				errors.HandleError(errors.NewNotFoundError("Project", args[0]), "Failed to list project comments")
				os.Exit(1)
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(map[string]any{
					"nodes":    parsed.Project.Comments.Nodes,
					"pageInfo": parsed.Project.Comments.PageInfo,
				})
				return
			}
			for _, raw := range parsed.Project.Comments.Nodes {
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

func newCommentAddCommand() *cobra.Command {
	var body string
	cmd := &cobra.Command{
		Use:   "add <projectId>",
		Short: "Add a comment to a project",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if body == "" {
				errors.HandleError(
					errors.NewValidationError("comment body is required", errors.WithSuggestion("Pass --body")),
					"Failed to add project comment",
				)
				os.Exit(1)
			}
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to add project comment")
				os.Exit(1)
			}
			id, err := resolveProjectRef(client, args[0])
			if err != nil {
				errors.HandleError(err, "Failed to add project comment")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), `
mutation CreateProjectComment($input: CommentCreateInput!) {
  commentCreate(input: $input) {
    success
    comment { id body }
  }
}`, map[string]any{"input": map[string]any{"projectId": id, "body": body}})
			if err != nil {
				errors.HandleError(err, "Failed to add project comment")
				os.Exit(1)
			}
			var parsed struct {
				CommentCreate struct {
					Success bool `json:"success"`
				} `json:"commentCreate"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil || !parsed.CommentCreate.Success {
				errors.HandleError(errors.NewCliError("Failed to add project comment"), "Failed to add project comment")
				os.Exit(1)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "✓ Comment added")
		},
	}
	cmd.Flags().StringVarP(&body, "body", "b", "", "Comment body")
	return cmd
}

func parseProjectPriority(priority string) (int, error) {
	switch strings.ToLower(strings.TrimSpace(priority)) {
	case "none":
		return 0, nil
	case "urgent":
		return 1, nil
	case "high":
		return 2, nil
	case "medium":
		return 3, nil
	case "low":
		return 4, nil
	default:
		return 0, errors.NewValidationError(
			fmt.Sprintf("Invalid priority: %s", priority),
			errors.WithSuggestion("Valid values: none, urgent, high, medium, low"),
		)
	}
}

func resolveTeamID(client *graphql.Client, key string) (string, error) {
	data, err := client.RequestRaw(context.Background(), `
query ResolveTeamForProject($reference: String!) {
  teams(filter: { or: [
    { key: { eqIgnoreCase: $reference } }
    { name: { eqIgnoreCase: $reference } }
  ]}) { nodes { id key } }
}`, map[string]any{"reference": key})
	if err != nil {
		return "", err
	}
	var parsed struct {
		Teams struct {
			Nodes []struct {
				ID string `json:"id"`
			} `json:"nodes"`
		} `json:"teams"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Teams.Nodes) == 0 {
		return "", errors.NewNotFoundError("Team", key)
	}
	return parsed.Teams.Nodes[0].ID, nil
}

func resolveProjectStatusID(client *graphql.Client, status string) (string, error) {
	mapping := map[string]string{
		"planned": "planned", "in progress": "started", "started": "started",
		"paused": "paused", "completed": "completed", "canceled": "canceled", "backlog": "backlog",
	}
	apiType, ok := mapping[strings.ToLower(status)]
	if !ok {
		return "", errors.NewValidationError(
			fmt.Sprintf("Invalid status: %s", status),
			errors.WithSuggestion("Valid values: planned, started, paused, completed, canceled, backlog"),
		)
	}
	data, err := client.RequestRaw(context.Background(), `
query GetProjectStatuses { projectStatuses { nodes { id name type } } }`, nil)
	if err != nil {
		return "", err
	}
	var parsed struct {
		ProjectStatuses struct {
			Nodes []struct {
				ID   string `json:"id"`
				Type string `json:"type"`
			} `json:"nodes"`
		} `json:"projectStatuses"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", err
	}
	for _, s := range parsed.ProjectStatuses.Nodes {
		if s.Type == apiType {
			return s.ID, nil
		}
	}
	return "", errors.NewNotFoundError("Project status", apiType)
}

func lookupUserID(client *graphql.Client, ref string) (string, error) {
	if ref == "@me" || strings.EqualFold(ref, "me") {
		data, err := client.RequestRaw(context.Background(), `query { viewer { id } }`, nil)
		if err != nil {
			return "", err
		}
		var parsed struct {
			Viewer struct {
				ID string `json:"id"`
			} `json:"viewer"`
		}
		if err := json.Unmarshal(data, &parsed); err != nil {
			return "", err
		}
		return parsed.Viewer.ID, nil
	}
	data, err := client.RequestRaw(context.Background(), `
query LookupUser($term: String!) {
  users(filter: { or: [
    { displayName: { eqIgnoreCase: $term } }
    { name: { eqIgnoreCase: $term } }
    { email: { eqIgnoreCase: $term } }
  ]}, first: 5) { nodes { id } }
}`, map[string]any{"term": ref})
	if err != nil {
		return "", err
	}
	var parsed struct {
		Users struct {
			Nodes []struct {
				ID string `json:"id"`
			} `json:"nodes"`
		} `json:"users"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Users.Nodes) == 0 {
		return "", errors.NewNotFoundError("User", ref)
	}
	return parsed.Users.Nodes[0].ID, nil
}

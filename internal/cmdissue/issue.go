package cmdissue

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/kongken/linear-cli/internal/config"
	"github.com/kongken/linear-cli/internal/display"
	"github.com/kongken/linear-cli/internal/errors"
	"github.com/kongken/linear-cli/internal/graphql"
	"github.com/kongken/linear-cli/internal/issues"
	"github.com/kongken/linear-cli/internal/linear"
	"github.com/kongken/linear-cli/internal/prompt"
	"github.com/kongken/linear-cli/internal/teams"
	"github.com/spf13/cobra"
)

// New returns the issue command group.
func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issue",
		Short: "Manage Linear issues",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(
		newIDCommand(),
		newTitleCommand(),
		newURLCommand(),
		newViewCommand(),
		newMineCommand(),
		newQueryCommand(),
		newCreateCommand(),
		newStartCommand(),
		newUpdateCommand(),
		newCommentCommand(),
		newArchiveCommand(),
		newDeleteCommand(),
		newDescribeCommand(),
		newLinkCommand(),
		newRelationCommand(),
		newPullRequestCommand(),
		newCommitsCommand(),
		newAttachCommand(),
		newAgentSessionCommand(),
	)

	return cmd
}

func newIDCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "id",
		Short: "Print the issue based on the current git branch",
		Run: func(cmd *cobra.Command, args []string) {
			id, err := linear.IssueIdentifier("")
			if err != nil {
				errors.HandleError(err, "Failed to get issue ID")
				os.Exit(1)
			}
			if id == "" {
				errors.HandleError(
					errors.NewValidationError(
						"Could not determine issue ID",
						errors.WithSuggestion(
							"Please provide an issue ID or run from a branch with an issue identifier.",
						),
					),
					"Failed to get issue ID",
				)
				os.Exit(1)
			}
			fmt.Fprintln(cmd.OutOrStdout(), id)
		},
	}
}

func newTitleCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "title [issueId]",
		Short: "Print the issue title",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			provided := ""
			if len(args) > 0 {
				provided = args[0]
			}
			runIssueField(cmd, provided, "Failed to get issue title", func(issue *linear.IssueTitleURL) string {
				return issue.Title
			})
		},
	}
}

func newURLCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "url [issueId]",
		Short: "Print the issue URL",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			provided := ""
			if len(args) > 0 {
				provided = args[0]
			}
			runIssueField(cmd, provided, "Failed to get issue URL", func(issue *linear.IssueTitleURL) string {
				return issue.URL
			})
		},
	}
}

func newMineCommand() *cobra.Command {
	var (
		team      string
		allStates bool
		limit     int
		sort      string
		jsonOut   bool
		unassigned, allAssignees bool
		project   string
		labels    []string
		createdAfter string
	)
	cmd := &cobra.Command{
		Use:     "mine",
		Aliases: []string{"list", "l"},
		Short:   "List your issues",
		Run: func(cmd *cobra.Command, args []string) {
			teamKey := strings.ToUpper(team)
			if teamKey == "" {
				if t, ok := config.GetOption("team_id"); ok {
					teamKey = strings.ToUpper(t)
				}
			}
			stateType := "unstarted"
			if allStates {
				stateType = ""
			}
			payload, err := issues.FetchMineIssues(context.Background(), issues.MineOptions{
				TeamKey:      teamKey,
				StateType:    stateType,
				Limit:        limit,
				Sort:         sort,
				Unassigned:   unassigned,
				AllAssignees: allAssignees,
				Project:      project,
				Labels:       labels,
				CreatedAfter: createdAfter,
			})
			if err != nil {
				errors.HandleError(err, "Failed to list issues")
				os.Exit(1)
			}
			if jsonOut {
				var pretty any
				if err := json.Unmarshal(payload, &pretty); err != nil {
					errors.HandleError(err, "Failed to list issues")
					os.Exit(1)
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(pretty)
				return
			}
			text, err := issues.FormatMineText(payload)
			if err != nil {
				errors.HandleError(err, "Failed to list issues")
				os.Exit(1)
			}
			fmt.Fprint(cmd.OutOrStdout(), text)
		},
	}
	cmd.Flags().StringVar(&team, "team", "", "Team key to list issues for")
	cmd.Flags().BoolVar(&allStates, "all-states", false, "Show issues from all states")
	cmd.Flags().BoolVarP(&allAssignees, "all-assignees", "A", false, "Show issues for all assignees")
	cmd.Flags().BoolVarP(&unassigned, "unassigned", "U", false, "Show only unassigned issues")
	cmd.Flags().StringVar(&project, "project", "", "Filter by project (UUID, slug, or name)")
	cmd.Flags().StringArrayVarP(&labels, "label", "l", nil, "Filter by label name (repeatable)")
	cmd.Flags().StringVar(&createdAfter, "created-after", "", "Only issues created on/after date (YYYY-MM-DD)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum number of issues to fetch")
	cmd.Flags().StringVar(&sort, "sort", "priority", "Sort order (priority or manual)")
	cmd.Flags().BoolVarP(&jsonOut, "json", "j", false, "Output as JSON connection {nodes,pageInfo}")
	return cmd
}

func newViewCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:     "view [issueId]",
		Aliases: []string{"v"},
		Short:   "View issue details (default) or open in browser/app",
		Args:    cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			provided := ""
			if len(args) > 0 {
				provided = args[0]
			}
			resolved, err := linear.IssueIdentifier(provided)
			if err != nil {
				errors.HandleError(err, "Failed to view issue")
				os.Exit(1)
			}
			if resolved == "" {
				errors.HandleError(
					errors.NewValidationError(
						"Could not determine issue ID",
						errors.WithSuggestion("Please provide an issue ID like 'ENG-123'."),
					),
					"Failed to view issue",
				)
				os.Exit(1)
			}
			raw, err := linear.FetchIssueDetailsJSON(context.Background(), resolved)
			if err != nil {
				errors.HandleError(err, "Failed to view issue")
				os.Exit(1)
			}
			if jsonOut {
				var pretty any
				if err := json.Unmarshal(raw, &pretty); err != nil {
					errors.HandleError(err, "Failed to view issue")
					os.Exit(1)
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if err := enc.Encode(pretty); err != nil {
					errors.HandleError(err, "Failed to view issue")
					os.Exit(1)
				}
				return
			}
			text, err := linear.FormatIssueDetailsText(raw)
			if err != nil {
				errors.HandleError(err, "Failed to view issue")
				os.Exit(1)
			}
			fmt.Fprint(cmd.OutOrStdout(), text)
		},
	}
	cmd.Flags().BoolVarP(&jsonOut, "json", "j", false, "Output issue data as JSON")
	return cmd
}

func runIssueField(cmd *cobra.Command, provided, failContext string, pick func(*linear.IssueTitleURL) string) {
	resolved, err := linear.IssueIdentifier(provided)
	if err != nil {
		errors.HandleError(err, failContext)
		os.Exit(1)
	}
	if resolved == "" {
		errors.HandleError(
			errors.NewValidationError(
				"Could not determine issue ID",
				errors.WithSuggestion("Please provide an issue ID like 'ENG-123'."),
			),
			failContext,
		)
		os.Exit(1)
	}
	issue, err := linear.FetchIssueTitleURL(context.Background(), resolved)
	if err != nil {
		errors.HandleError(err, failContext)
		os.Exit(1)
	}
	fmt.Fprintln(cmd.OutOrStdout(), pick(issue))
}

func newQueryCommand() *cobra.Command {
	var (
		teams           []string
		allTeams        bool
		state           string
		assignee        string
		assigneeMe      bool
		unassigned      bool
		project         string
		projectLabel    string
		labels          []string
		createdAfter    string
		updatedAfter    string
		includeArchived bool
		search          string
		limit           int
		sort            string
		jsonOut         bool
	)
	cmd := &cobra.Command{
		Use:     "query",
		Aliases: []string{"q"},
		Short:   "Query issues with structured filters",
		Run: func(cmd *cobra.Command, args []string) {
			teamKeys := make([]string, 0, len(teams))
			for _, t := range teams {
				teamKeys = append(teamKeys, strings.ToUpper(t))
			}
			if !allTeams && len(teamKeys) == 0 {
				if t, ok := config.GetOption("team_id"); ok && t != "" {
					teamKeys = append(teamKeys, strings.ToUpper(t))
				}
			}
			payload, err := issues.FetchQueryIssues(context.Background(), issues.QueryOptions{
				TeamKeys:        teamKeys,
				AllTeams:        allTeams,
				StateType:       state,
				Assignee:        assignee,
				AssigneeMe:      assigneeMe,
				Unassigned:      unassigned,
				Project:         project,
				ProjectLabel:    projectLabel,
				Labels:          labels,
				CreatedAfter:    createdAfter,
				UpdatedAfter:    updatedAfter,
				IncludeArchived: includeArchived,
				Search:          search,
				Limit:           limit,
				Sort:            sort,
			})
			if err != nil {
				errors.HandleError(err, "Failed to query issues")
				os.Exit(1)
			}
			if jsonOut {
				var pretty any
				_ = json.Unmarshal(payload, &pretty)
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(pretty)
				return
			}
			text, err := issues.FormatQueryText(payload)
			if err != nil {
				errors.HandleError(err, "Failed to query issues")
				os.Exit(1)
			}
			fmt.Fprint(cmd.OutOrStdout(), text)
		},
	}
	cmd.Flags().StringArrayVar(&teams, "team", nil, "Filter by team key (repeatable)")
	cmd.Flags().BoolVar(&allTeams, "all-teams", false, "Query across all teams")
	cmd.Flags().StringVarP(&state, "state", "s", "", "Filter by workflow state type")
	cmd.Flags().StringVar(&assignee, "assignee", "", "Filter by assignee (username)")
	cmd.Flags().BoolVar(&assigneeMe, "mine", false, "Only issues assigned to me")
	cmd.Flags().BoolVarP(&unassigned, "unassigned", "U", false, "Only unassigned issues")
	cmd.Flags().StringVar(&project, "project", "", "Filter by project (UUID, slug, or name)")
	cmd.Flags().StringVar(&projectLabel, "project-label", "", "Filter by project label name")
	cmd.Flags().StringArrayVarP(&labels, "label", "l", nil, "Filter by label name (repeatable)")
	cmd.Flags().StringVar(&createdAfter, "created-after", "", "Only issues created on/after date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&updatedAfter, "updated-after", "", "Only issues updated on/after date (YYYY-MM-DD)")
	cmd.Flags().BoolVar(&includeArchived, "include-archived", false, "Include archived issues")
	cmd.Flags().StringVar(&search, "search", "", "Full-text search term")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum number of issues to fetch")
	cmd.Flags().StringVar(&sort, "sort", "priority", "Sort order (priority or manual)")
	cmd.Flags().BoolVarP(&jsonOut, "json", "j", false, "Output as JSON connection")
	return cmd
}

func newCreateCommand() *cobra.Command {
	var (
		title, team, description, descriptionFile string
		assignee, parent, project, state          string
		milestone, cycle, template                string
		priority, estimate                        int
		labels                                    []string
		assignSelf, start, noInteractive          bool
		noUseDefaultTemplate                      bool
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an issue",
		Run: func(cmd *cobra.Command, args []string) {
			teamKey := strings.ToUpper(team)
			if teamKey == "" {
				if t, ok := config.GetOption("team_id"); ok {
					teamKey = strings.ToUpper(t)
				}
			}
			if description != "" && descriptionFile != "" {
				errors.HandleError(
					errors.NewValidationError("Cannot specify both --description and --description-file"),
					"Failed to create issue",
				)
				os.Exit(1)
			}

			interactive := !noInteractive && prompt.IsInteractive() && title == "" && template == ""
			var wizStart bool
			titleVal := title
			desc := description
			assigneeRef := assignee
			projectRef := project
			stateRef := state
			var pri *float64
			if cmd.Flags().Changed("priority") {
				p := float64(priority)
				pri = &p
			}
			var est *float64
			if cmd.Flags().Changed("estimate") {
				e := float64(estimate)
				est = &e
			}
			labelVals := append([]string{}, labels...)

			if interactive {
				wiz, err := runInteractiveIssueCreate(teamKey, project)
				if err != nil {
					errors.HandleError(err, "Failed to create issue")
					os.Exit(1)
				}
				titleVal = wiz.Title
				teamKey = wiz.TeamKey
				if wiz.Description != "" {
					desc = wiz.Description
				}
				if wiz.Priority != nil {
					pri = wiz.Priority
				}
				if wiz.Estimate != nil {
					est = wiz.Estimate
				}
				if wiz.Assignee != "" {
					assigneeRef = wiz.Assignee
				}
				if wiz.Project != "" {
					projectRef = wiz.Project
				}
				if wiz.State != "" {
					stateRef = wiz.State
				}
				if len(wiz.Labels) > 0 {
					labelVals = append(labelVals, wiz.Labels...)
				}
				wizStart = wiz.Start
			} else if titleVal == "" && template == "" && !noInteractive {
				t, err := prompt.Text("Issue title", "")
				if err != nil {
					errors.HandleError(err, "Failed to create issue")
					os.Exit(1)
				}
				titleVal = t
			}

			if teamKey == "" {
				errors.HandleError(
					errors.NewValidationError(
						"team is required",
						errors.WithSuggestion("Pass --team or configure team_id via `linear config`."),
					),
					"Failed to create issue",
				)
				os.Exit(1)
			}
			if descriptionFile != "" {
				d, err := issues.ReadDescriptionFile(descriptionFile)
				if err != nil {
					errors.HandleError(err, "Failed to create issue")
					os.Exit(1)
				}
				desc = d
			}
			if assignSelf && assigneeRef == "" {
				assigneeRef = "self"
			}
			issue, err := issues.CreateIssue(context.Background(), issues.CreateInput{
				Title:                titleVal,
				TeamKey:              teamKey,
				Description:          desc,
				Priority:             pri,
				Estimate:             est,
				Assignee:             assigneeRef,
				Parent:               parent,
				Project:              projectRef,
				State:                stateRef,
				Milestone:            milestone,
				Cycle:                cycle,
				Labels:               labelVals,
				Template:             template,
				NoUseDefaultTemplate: noUseDefaultTemplate,
			})
			if err != nil {
				errors.HandleError(err, "Failed to create issue")
				os.Exit(1)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Created issue %s: %s\n", issue.Identifier, issue.Title)
			fmt.Fprintln(cmd.OutOrStdout(), issue.URL)
			if start || wizStart {
				if err := issues.StartWork(context.Background(), issue.Identifier, "", ""); err != nil {
					errors.HandleError(err, "Failed to start issue")
					os.Exit(1)
				}
			}
		},
	}
	cmd.Flags().StringVarP(&title, "title", "t", "", "Title of the issue")
	cmd.Flags().StringVar(&team, "team", "", "Team key, name, or ID")
	cmd.Flags().StringVarP(&description, "description", "d", "", "Issue description")
	cmd.Flags().StringVar(&descriptionFile, "description-file", "", "Read description from file")
	cmd.Flags().IntVarP(&priority, "priority", "p", 0, "Priority (0-4)")
	cmd.Flags().IntVar(&estimate, "estimate", 0, "Points estimate")
	cmd.Flags().StringVarP(&assignee, "assignee", "a", "", "Assignee (self/@me/username)")
	cmd.Flags().BoolVar(&assignSelf, "assign-self", false, "Assign the issue to yourself")
	cmd.Flags().StringVar(&parent, "parent", "", "Parent issue identifier")
	cmd.Flags().StringVar(&project, "project", "", "Project (UUID, slug, or name)")
	cmd.Flags().StringVarP(&state, "state", "s", "", "Workflow state (name or type)")
	cmd.Flags().StringVar(&milestone, "milestone", "", "Project milestone")
	cmd.Flags().StringVar(&cycle, "cycle", "", "Cycle (number, name, active/next/previous)")
	cmd.Flags().StringArrayVarP(&labels, "label", "l", nil, "Label name (repeatable)")
	cmd.Flags().StringVar(&template, "template", "", "Issue template by name or ID")
	cmd.Flags().BoolVar(&noUseDefaultTemplate, "no-use-default-template", false, "Do not use the team's default template")
	cmd.Flags().BoolVar(&start, "start", false, "Start the issue after creation")
	cmd.Flags().BoolVar(&noInteractive, "no-interactive", false, "Disable interactive prompts")
	return cmd
}

func newStartCommand() *cobra.Command {
	var fromRef, branch string
	var allAssignees, unassigned bool
	cmd := &cobra.Command{
		Use:   "start [issueId]",
		Short: "Start working on an issue",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if allAssignees && unassigned {
				errors.HandleError(
					errors.NewValidationError("Cannot specify both --all-assignees and --unassigned"),
					"Failed to start issue",
				)
				os.Exit(1)
			}
			var resolved string
			if len(args) > 0 {
				var err error
				resolved, err = linear.IssueIdentifier(args[0])
				if err != nil {
					errors.HandleError(err, "Failed to start issue")
					os.Exit(1)
				}
			}
			if resolved == "" {
				teamKey := ""
				if t, ok := config.GetOption("team_id"); ok {
					teamKey = strings.ToUpper(t)
				}
				if teamKey == "" {
					errors.HandleError(
						errors.NewValidationError("Could not determine team ID"),
						"Failed to start issue",
					)
					os.Exit(1)
				}
				payload, err := issues.FetchMineIssues(context.Background(), issues.MineOptions{
					TeamKey:      teamKey,
					StateType:    "unstarted",
					Limit:        50,
					Unassigned:   unassigned,
					AllAssignees: allAssignees,
				})
				if err != nil {
					errors.HandleError(err, "Failed to start issue")
					os.Exit(1)
				}
				summaries, err := issues.ParseIssueSummaries(payload)
				if err != nil {
					errors.HandleError(err, "Failed to start issue")
					os.Exit(1)
				}
				if len(summaries) == 0 {
					errors.HandleError(
						errors.NewNotFoundError("Unstarted issues", teamKey),
						"Failed to start issue",
					)
					os.Exit(1)
				}
				opts := make([]prompt.Option, 0, len(summaries))
				for _, s := range summaries {
					opts = append(opts, prompt.Option{
						Label: fmt.Sprintf("%s %s: %s", display.PriorityDisplay(s.Priority), s.Identifier, s.Title),
						Value: s.Identifier,
					})
				}
				selected, err := prompt.Select("Select an issue to start:", opts)
				if err != nil {
					errors.HandleError(err, "Failed to start issue")
					os.Exit(1)
				}
				resolved = selected
			}
			if resolved == "" {
				errors.HandleError(
					errors.NewValidationError("Could not determine issue ID"),
					"Failed to start issue",
				)
				os.Exit(1)
			}
			if err := issues.StartWork(context.Background(), resolved, fromRef, branch); err != nil {
				errors.HandleError(err, "Failed to start issue")
				os.Exit(1)
			}
		},
	}
	cmd.Flags().BoolVarP(&allAssignees, "all-assignees", "A", false, "Show issues for all assignees")
	cmd.Flags().BoolVarP(&unassigned, "unassigned", "U", false, "Show only unassigned issues")
	cmd.Flags().StringVarP(&fromRef, "from-ref", "f", "", "Git ref to create new branch from")
	cmd.Flags().StringVarP(&branch, "branch", "b", "", "Custom branch name")
	return cmd
}

func newUpdateCommand() *cobra.Command {
	var (
		title, description, descriptionFile string
		assignee, parent, project, state    string
		milestone, cycle, team              string
		priority, estimate                  int
		labels                              []string
		addLabels, removeLabels             []string
		unassign, clearDueDate              bool
		clearParent, clearProject           bool
		clearMilestone, clearCycle          bool
		clearEstimate                       bool
		dueDate                             string
	)
	cmd := &cobra.Command{
		Use:   "update [issueId]",
		Short: "Update a Linear issue",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			provided := ""
			if len(args) > 0 {
				provided = args[0]
			}
			resolved, err := linear.IssueIdentifier(provided)
			if err != nil {
				errors.HandleError(err, "Failed to update issue")
				os.Exit(1)
			}
			if resolved == "" {
				errors.HandleError(
					errors.NewValidationError(
						"Could not determine issue ID",
						errors.WithSuggestion("Please provide an issue ID like 'ENG-123'."),
					),
					"Failed to update issue",
				)
				os.Exit(1)
			}
			if description != "" && descriptionFile != "" {
				errors.HandleError(
					errors.NewValidationError("Cannot specify both --description and --description-file"),
					"Failed to update issue",
				)
				os.Exit(1)
			}
			if len(labels) > 0 && (len(addLabels) > 0 || len(removeLabels) > 0) {
				errors.HandleError(
					errors.NewValidationError(
						"Cannot combine --label with --add-label or --remove-label",
						errors.WithSuggestion("--label replaces the entire label set; use --add-label/--remove-label alone for incremental changes."),
					),
					"Failed to update issue",
				)
				os.Exit(1)
			}
			if team != "" && (len(addLabels) > 0 || len(removeLabels) > 0) {
				errors.HandleError(
					errors.NewValidationError(
						"Cannot combine --team with --add-label or --remove-label",
					),
					"Failed to update issue",
				)
				os.Exit(1)
			}
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to update issue")
				os.Exit(1)
			}
			input := map[string]any{}
			if cmd.Flags().Changed("title") {
				input["title"] = title
			}
			if descriptionFile != "" {
				d, err := issues.ReadDescriptionFile(descriptionFile)
				if err != nil {
					errors.HandleError(err, "Failed to update issue")
					os.Exit(1)
				}
				input["description"] = d
			} else if cmd.Flags().Changed("description") {
				input["description"] = description
			}
			if cmd.Flags().Changed("priority") {
				input["priority"] = priority
			}
			if clearEstimate {
				input["estimate"] = nil
			} else if cmd.Flags().Changed("estimate") {
				input["estimate"] = estimate
			}
			if clearDueDate {
				input["dueDate"] = nil
			} else if dueDate != "" {
				input["dueDate"] = dueDate
			}
			if unassign {
				input["assigneeId"] = nil
			} else if assignee != "" {
				id, err := issues.ResolveUserID(context.Background(), client, assignee)
				if err != nil {
					errors.HandleError(err, "Failed to update issue")
					os.Exit(1)
				}
				input["assigneeId"] = id
			}
			if clearParent {
				input["parentId"] = nil
			} else if parent != "" {
				pid, err := issues.ResolveIssueUUID(context.Background(), client, parent)
				if err != nil {
					errors.HandleError(err, "Failed to update issue")
					os.Exit(1)
				}
				input["parentId"] = pid
			}
			var projectID string
			if clearProject {
				input["projectId"] = nil
			} else if project != "" {
				projectID, err = issues.ResolveProjectID(context.Background(), client, project)
				if err != nil {
					errors.HandleError(err, "Failed to update issue")
					os.Exit(1)
				}
				input["projectId"] = projectID
			}
			if team != "" {
				t, err := teams.Resolve(context.Background(), team)
				if err != nil {
					errors.HandleError(err, "Failed to update issue")
					os.Exit(1)
				}
				input["teamId"] = t.ID
			}
			if state != "" {
				teamID := ""
				if team != "" {
					t, err := teams.Resolve(context.Background(), team)
					if err != nil {
						errors.HandleError(err, "Failed to update issue")
						os.Exit(1)
					}
					teamID = t.ID
				} else {
					data, err := client.RequestRaw(context.Background(), `query($id: String!) { issue(id: $id) { team { id key } } }`, map[string]any{"id": resolved})
					if err != nil {
						errors.HandleError(err, "Failed to update issue")
						os.Exit(1)
					}
					var iss struct {
						Issue *struct {
							Team struct {
								ID  string `json:"id"`
								Key string `json:"key"`
							} `json:"team"`
						} `json:"issue"`
					}
					_ = json.Unmarshal(data, &iss)
					if iss.Issue == nil {
						errors.HandleError(errors.NewNotFoundError("Issue", resolved), "Failed to update issue")
						os.Exit(1)
					}
					teamID = iss.Issue.Team.ID
				}
				sid, err := issues.ResolveWorkflowStateID(context.Background(), client, teamID, state)
				if err != nil {
					errors.HandleError(err, "Failed to update issue")
					os.Exit(1)
				}
				input["stateId"] = sid
			}
			if clearMilestone {
				input["projectMilestoneId"] = nil
			} else if milestone != "" {
				mid, err := issues.ResolveMilestoneID(context.Background(), client, milestone, projectID)
				if err != nil {
					errors.HandleError(err, "Failed to update issue")
					os.Exit(1)
				}
				input["projectMilestoneId"] = mid
			}
			if clearCycle {
				input["cycleId"] = nil
			} else if cycle != "" {
				data, err := client.RequestRaw(context.Background(), `query($id: String!) { issue(id: $id) { team { id } } }`, map[string]any{"id": resolved})
				if err != nil {
					errors.HandleError(err, "Failed to update issue")
					os.Exit(1)
				}
				var iss struct {
					Issue *struct {
						Team struct {
							ID string `json:"id"`
						} `json:"team"`
					} `json:"issue"`
				}
				_ = json.Unmarshal(data, &iss)
				if iss.Issue == nil {
					errors.HandleError(errors.NewNotFoundError("Issue", resolved), "Failed to update issue")
					os.Exit(1)
				}
				cid, err := issues.ResolveCycleID(context.Background(), client, iss.Issue.Team.ID, cycle)
				if err != nil {
					errors.HandleError(err, "Failed to update issue")
					os.Exit(1)
				}
				input["cycleId"] = cid
			}
			if len(labels) > 0 {
				data, err := client.RequestRaw(context.Background(), `query($id: String!) { issue(id: $id) { team { key } } }`, map[string]any{"id": resolved})
				if err != nil {
					errors.HandleError(err, "Failed to update issue")
					os.Exit(1)
				}
				var iss struct {
					Issue *struct {
						Team struct {
							Key string `json:"key"`
						} `json:"team"`
					} `json:"issue"`
				}
				_ = json.Unmarshal(data, &iss)
				teamKey := ""
				if iss.Issue != nil {
					teamKey = iss.Issue.Team.Key
				}
				ids, err := issues.ResolveLabelIDs(context.Background(), client, teamKey, labels)
				if err != nil {
					errors.HandleError(err, "Failed to update issue")
					os.Exit(1)
				}
				input["labelIds"] = ids
			}
			if len(addLabels) > 0 || len(removeLabels) > 0 {
				data, err := client.RequestRaw(context.Background(), `query($id: String!) { issue(id: $id) { team { key } } }`, map[string]any{"id": resolved})
				if err != nil {
					errors.HandleError(err, "Failed to update issue")
					os.Exit(1)
				}
				var iss struct {
					Issue *struct {
						Team struct {
							Key string `json:"key"`
						} `json:"team"`
					} `json:"issue"`
				}
				_ = json.Unmarshal(data, &iss)
				teamKey := ""
				if iss.Issue != nil {
					teamKey = iss.Issue.Team.Key
				}
				if len(addLabels) > 0 {
					ids, err := issues.ResolveLabelIDs(context.Background(), client, teamKey, addLabels)
					if err != nil {
						errors.HandleError(err, "Failed to update issue")
						os.Exit(1)
					}
					input["addedLabelIds"] = ids
				}
				if len(removeLabels) > 0 {
					ids, err := issues.ResolveLabelIDs(context.Background(), client, teamKey, removeLabels)
					if err != nil {
						errors.HandleError(err, "Failed to update issue")
						os.Exit(1)
					}
					input["removedLabelIds"] = ids
				}
			}
			if len(input) == 0 {
				errors.HandleError(
					errors.NewValidationError("No update fields provided"),
					"Failed to update issue",
				)
				os.Exit(1)
			}
			issue, err := issues.UpdateIssue(context.Background(), resolved, input)
			if err != nil {
				errors.HandleError(err, "Failed to update issue")
				os.Exit(1)
			}
			var out struct {
				Identifier string `json:"identifier"`
				URL        string `json:"url"`
			}
			_ = json.Unmarshal(issue, &out)
			fmt.Fprintf(cmd.OutOrStdout(), "Updated %s\n%s\n", out.Identifier, out.URL)
		},
	}
	cmd.Flags().StringVarP(&title, "title", "t", "", "New title")
	cmd.Flags().StringVarP(&description, "description", "d", "", "New description")
	cmd.Flags().StringVar(&descriptionFile, "description-file", "", "Read description from file")
	cmd.Flags().IntVarP(&priority, "priority", "p", 0, "Priority 0-4")
	cmd.Flags().IntVar(&estimate, "estimate", 0, "Points estimate")
	cmd.Flags().BoolVar(&clearEstimate, "clear-estimate", false, "Clear estimate")
	cmd.Flags().StringVarP(&assignee, "assignee", "a", "", "Assign to self/@me/username")
	cmd.Flags().BoolVar(&unassign, "unassign", false, "Clear assignee")
	cmd.Flags().StringVar(&dueDate, "due-date", "", "Due date (YYYY-MM-DD)")
	cmd.Flags().BoolVar(&clearDueDate, "clear-due-date", false, "Clear due date")
	cmd.Flags().StringVar(&parent, "parent", "", "Parent issue")
	cmd.Flags().BoolVar(&clearParent, "clear-parent", false, "Clear parent")
	cmd.Flags().StringVar(&team, "team", "", "Move to team")
	cmd.Flags().StringVar(&project, "project", "", "Assign project")
	cmd.Flags().BoolVar(&clearProject, "clear-project", false, "Clear project")
	cmd.Flags().StringVarP(&state, "state", "s", "", "Workflow state")
	cmd.Flags().StringVar(&milestone, "milestone", "", "Project milestone")
	cmd.Flags().BoolVar(&clearMilestone, "clear-milestone", false, "Clear milestone")
	cmd.Flags().StringVar(&cycle, "cycle", "", "Cycle")
	cmd.Flags().BoolVar(&clearCycle, "clear-cycle", false, "Clear cycle")
	cmd.Flags().StringArrayVarP(&labels, "label", "l", nil, "Replace labels (repeatable)")
	cmd.Flags().StringArrayVar(&addLabels, "add-label", nil, "Add label by name (repeatable)")
	cmd.Flags().StringArrayVar(&removeLabels, "remove-label", nil, "Remove label by name (repeatable)")
	return cmd
}

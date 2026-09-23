package cmdissue

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/kongken/linear-cli/internal/config"
	"github.com/kongken/linear-cli/internal/display"
	"github.com/kongken/linear-cli/internal/editor"
	"github.com/kongken/linear-cli/internal/errors"
	"github.com/kongken/linear-cli/internal/prompt"
	"github.com/kongken/linear-cli/internal/teams"
)

// interactiveCreateResult holds values gathered by the create wizard.
type interactiveCreateResult struct {
	Title       string
	TeamKey     string
	Description string
	Priority    *float64
	Estimate    *float64
	Assignee    string
	Project     string
	State       string
	Labels      []string
	Start       bool
}

func runInteractiveIssueCreate(seedTeam, seedProject string) (*interactiveCreateResult, error) {
	title, err := prompt.Text("What's the title of your issue?", "")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(title) == "" {
		return nil, errors.NewValidationError("title is required")
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
		selected, err := prompt.Select("Which team should this issue belong to?", opts)
		if err != nil {
			return nil, err
		}
		teamKey = selected
	}

	descPrompt := "Description"
	if edName := editor.DisplayName(); edName != "" {
		descPrompt = fmt.Sprintf("Description [(e) to launch %s]", edName)
	}
	description, err := prompt.Text(descPrompt, "")
	if err != nil {
		return nil, err
	}
	finalDescription := strings.TrimSpace(description)
	if finalDescription == "e" {
		if edName := editor.DisplayName(); edName != "" {
			fmt.Printf("Opening %s...\n", edName)
			edited, err := editor.Open()
			if err != nil {
				return nil, err
			}
			finalDescription = edited
		} else {
			return nil, errors.NewValidationError(
				"No editor found",
				errors.WithSuggestion("Set EDITOR environment variable or configure git editor with: git config --global core.editor <editor>"),
			)
		}
	}

	result := &interactiveCreateResult{
		Title:       strings.TrimSpace(title),
		TeamKey:     teamKey,
		Description: finalDescription,
		Project:     seedProject,
	}

	if mode, ok := config.GetOption("issue_create_assign_self"); ok && mode == "always" {
		result.Assignee = "self"
	}

	next, err := prompt.Select("What's next?", []prompt.Option{
		{Label: "Submit issue", Value: "submit"},
		{Label: "Add more fields", Value: "more_fields"},
	})
	if err != nil {
		return nil, err
	}

	if next == "more_fields" {
		if err := promptAdditionalCreateFields(result); err != nil {
			return nil, err
		}
	}

	startChoice, err := prompt.Select(
		"Start working on this issue now? (creates branch and updates status)",
		[]prompt.Option{
			{Label: "No", Value: "no"},
			{Label: "Yes", Value: "yes"},
		},
	)
	if err != nil {
		return nil, err
	}
	result.Start = startChoice == "yes"
	return result, nil
}

func promptAdditionalCreateFields(result *interactiveCreateResult) error {
	choices, err := prompt.MultiSelect("Select additional fields to configure", []prompt.Option{
		{Label: "Assignee", Value: "assignee"},
		{Label: "Priority", Value: "priority"},
		{Label: "Estimate", Value: "estimate"},
		{Label: "State", Value: "state"},
		{Label: "Project", Value: "project"},
		{Label: "Label", Value: "label"},
	})
	if err != nil {
		return err
	}
	for _, field := range choices {
		switch field {
		case "assignee":
			choice, err := prompt.Select("Assignee", []prompt.Option{
				{Label: "Assign to myself", Value: "self"},
				{Label: "Leave unassigned", Value: ""},
			})
			if err != nil {
				return err
			}
			result.Assignee = choice
		case "priority":
			choice, err := prompt.Select("Priority", []prompt.Option{
				{Label: display.PriorityDisplay(0) + " No priority", Value: "0"},
				{Label: display.PriorityDisplay(1) + " Urgent", Value: "1"},
				{Label: display.PriorityDisplay(2) + " High", Value: "2"},
				{Label: display.PriorityDisplay(3) + " Medium", Value: "3"},
				{Label: display.PriorityDisplay(4) + " Low", Value: "4"},
			})
			if err != nil {
				return err
			}
			p, _ := strconv.ParseFloat(choice, 64)
			result.Priority = &p
		case "estimate":
			raw, err := prompt.Text("Estimate (points)", "")
			if err != nil {
				return err
			}
			if raw != "" {
				e, err := strconv.ParseFloat(raw, 64)
				if err != nil {
					return errors.NewValidationError("estimate must be a number")
				}
				result.Estimate = &e
			}
		case "state":
			s, err := prompt.Text("Workflow state (name or type)", "")
			if err != nil {
				return err
			}
			result.State = strings.TrimSpace(s)
		case "project":
			p, err := prompt.Text("Project (UUID, slug, or name)", "")
			if err != nil {
				return err
			}
			result.Project = strings.TrimSpace(p)
		case "label":
			l, err := prompt.Text("Label name", "")
			if err != nil {
				return err
			}
			if l = strings.TrimSpace(l); l != "" {
				result.Labels = append(result.Labels, l)
			}
		}
	}
	return nil
}

package cmdtemplate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/kongken/linear-cli/internal/errors"
	"github.com/kongken/linear-cli/internal/graphql"
	"github.com/kongken/linear-cli/internal/teams"
	"github.com/spf13/cobra"
)

const getTemplatesQuery = `
query GetTemplates {
  templates {
    id name description type icon color hasFormFields
    lastAppliedAt sortOrder createdAt updatedAt
    team { id key name }
    creator { id name }
  }
}
`

const getTemplateQuery = `
query GetTemplate($id: String!) {
  template(id: $id) {
    id name description type icon color hasFormFields
    lastAppliedAt sortOrder createdAt updatedAt
    team { id key name }
    templateData
  }
}
`

// New returns the template command group.
func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "template",
		Short: "Manage templates",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newListCommand(), newViewCommand())
	return cmd
}

func newListCommand() *cobra.Command {
	var (
		typ, team string
		jsonOut   bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List templates",
		Run: func(cmd *cobra.Command, args []string) {
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to list templates")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), getTemplatesQuery, nil)
			if err != nil {
				errors.HandleError(err, "Failed to list templates")
				os.Exit(1)
			}
			var parsed struct {
				Templates []json.RawMessage `json:"templates"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil {
				errors.HandleError(err, "Failed to list templates")
				os.Exit(1)
			}
			var teamID string
			if team != "" {
				t, err := teams.Resolve(context.Background(), team)
				if err != nil {
					errors.HandleError(err, "Failed to list templates")
					os.Exit(1)
				}
				teamID = t.ID
			}
			filtered := make([]json.RawMessage, 0, len(parsed.Templates))
			for _, raw := range parsed.Templates {
				var t struct {
					Type string `json:"type"`
					Team *struct {
						ID string `json:"id"`
					} `json:"team"`
				}
				_ = json.Unmarshal(raw, &t)
				if typ != "" && !strings.EqualFold(t.Type, typ) {
					continue
				}
				// Without --team: all. With --team: workspace templates + that team's.
				if teamID != "" && t.Team != nil && t.Team.ID != teamID {
					continue
				}
				filtered = append(filtered, raw)
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(filtered)
				return
			}
			if len(filtered) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No templates found.")
				return
			}
			for _, raw := range filtered {
				var t struct {
					ID   string `json:"id"`
					Name string `json:"name"`
					Type string `json:"type"`
					Team *struct {
						Key string `json:"key"`
					} `json:"team"`
				}
				_ = json.Unmarshal(raw, &t)
				scope := "workspace"
				if t.Team != nil {
					scope = t.Team.Key
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\n", t.Type, scope, t.Name, t.ID)
			}
		},
	}
	cmd.Flags().StringVar(&typ, "type", "", "Only templates of this type (issue, project, or document)")
	cmd.Flags().StringVar(&team, "team", "", "Team key, name, or ID")
	cmd.Flags().BoolVarP(&jsonOut, "json", "j", false, "Output as JSON")
	return cmd
}

func newViewCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "view <templateId>",
		Short: "View a template",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to view template")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), getTemplateQuery, map[string]any{"id": args[0]})
			if err != nil {
				errors.HandleError(err, "Failed to view template")
				os.Exit(1)
			}
			var parsed struct {
				Template json.RawMessage `json:"template"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil || len(parsed.Template) == 0 || string(parsed.Template) == "null" {
				errors.HandleError(errors.NewNotFoundError("Template", args[0]), "Failed to view template")
				os.Exit(1)
			}
			if jsonOut {
				var pretty any
				_ = json.Unmarshal(parsed.Template, &pretty)
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(pretty)
				return
			}
			var t struct {
				Name        string `json:"name"`
				Type        string `json:"type"`
				Description string `json:"description"`
			}
			_ = json.Unmarshal(parsed.Template, &t)
			fmt.Fprintf(cmd.OutOrStdout(), "%s (%s)\n%s\n", t.Name, t.Type, t.Description)
		},
	}
	cmd.Flags().BoolVarP(&jsonOut, "json", "j", false, "Output as JSON")
	return cmd
}

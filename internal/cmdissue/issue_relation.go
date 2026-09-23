package cmdissue

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/kongken/linear-cli/internal/errors"
	"github.com/kongken/linear-cli/internal/graphql"
	"github.com/spf13/cobra"
)

const relationCreateMutation = `
mutation IssueRelationCreate($input: IssueRelationCreateInput!) {
  issueRelationCreate(input: $input) {
    success
    issueRelation { id type }
  }
}
`

func newRelationCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "relation",
		Short: "Manage issue relations",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newRelationAddCommand())
	cmd.AddCommand(newRelationDeleteCommand())
	cmd.AddCommand(newRelationListCommand())
	return cmd
}

func newRelationAddCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "add <issueId> <relationType> <relatedIssueId>",
		Short: "Add a relation between two issues",
		Args:  cobra.ExactArgs(3),
		Run: func(cmd *cobra.Command, args []string) {
			relType := strings.ToLower(args[1])
			allowed := map[string]string{
				"blocks": "blocks", "blocked-by": "blocks",
				"related": "related", "duplicate": "duplicate",
			}
			apiType, ok := allowed[relType]
			if !ok {
				errors.HandleError(
					errors.NewValidationError(
						fmt.Sprintf("Invalid relation type: %s", args[1]),
						errors.WithSuggestion("Must be one of: blocks, blocked-by, related, duplicate"),
					),
					"Failed to add relation",
				)
				os.Exit(1)
			}
			issueA := mustResolveIssue(args[0], "Failed to add relation")
			issueB := mustResolveIssue(args[2], "Failed to add relation")
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to add relation")
				os.Exit(1)
			}
			idA, err := issueUUID(client, issueA)
			if err != nil {
				errors.HandleError(err, "Failed to add relation")
				os.Exit(1)
			}
			idB, err := issueUUID(client, issueB)
			if err != nil {
				errors.HandleError(err, "Failed to add relation")
				os.Exit(1)
			}
			issueID, relatedID := idA, idB
			if relType == "blocked-by" {
				issueID, relatedID = idB, idA
			}
			data, err := client.RequestRaw(context.Background(), relationCreateMutation, map[string]any{
				"input": map[string]any{
					"issueId":        issueID,
					"relatedIssueId": relatedID,
					"type":           apiType,
				},
			})
			if err != nil {
				errors.HandleError(err, "Failed to add relation")
				os.Exit(1)
			}
			var parsed struct {
				IssueRelationCreate struct {
					Success bool `json:"success"`
				} `json:"issueRelationCreate"`
			}
			_ = json.Unmarshal(data, &parsed)
			if !parsed.IssueRelationCreate.Success {
				errors.HandleError(errors.NewCliError("Failed to create relation"), "Failed to add relation")
				os.Exit(1)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Added %s relation between %s and %s\n", relType, issueA, issueB)
		},
	}
}

func newRelationDeleteCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <issueId> <relationType> <relatedIssueId>",
		Short: "Delete a relation between two issues",
		Args:  cobra.ExactArgs(3),
		Run: func(cmd *cobra.Command, args []string) {
			relType := strings.ToLower(args[1])
			allowed := map[string]string{
				"blocks": "blocks", "blocked-by": "blocks",
				"related": "related", "duplicate": "duplicate",
			}
			apiType, ok := allowed[relType]
			if !ok {
				errors.HandleError(
					errors.NewValidationError(
						fmt.Sprintf("Invalid relation type: %s", args[1]),
						errors.WithSuggestion("Must be one of: blocks, blocked-by, related, duplicate"),
					),
					"Failed to delete relation",
				)
				os.Exit(1)
			}
			issueA := mustResolveIssue(args[0], "Failed to delete relation")
			issueB := mustResolveIssue(args[2], "Failed to delete relation")
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to delete relation")
				os.Exit(1)
			}
			idA, err := issueUUID(client, issueA)
			if err != nil {
				errors.HandleError(err, "Failed to delete relation")
				os.Exit(1)
			}
			idB, err := issueUUID(client, issueB)
			if err != nil {
				errors.HandleError(err, "Failed to delete relation")
				os.Exit(1)
			}
			fromID, toID := idA, idB
			if relType == "blocked-by" {
				fromID, toID = idB, idA
			}
			data, err := client.RequestRaw(context.Background(), `
query FindIssueRelation($issueId: String!) {
  issue(id: $issueId) {
    relations {
      nodes {
        id
        type
        relatedIssue { id }
      }
    }
  }
}`, map[string]any{"issueId": fromID})
			if err != nil {
				errors.HandleError(err, "Failed to delete relation")
				os.Exit(1)
			}
			var find struct {
				Issue *struct {
					Relations struct {
						Nodes []struct {
							ID           string `json:"id"`
							Type         string `json:"type"`
							RelatedIssue struct {
								ID string `json:"id"`
							} `json:"relatedIssue"`
						} `json:"nodes"`
					} `json:"relations"`
				} `json:"issue"`
			}
			if err := json.Unmarshal(data, &find); err != nil || find.Issue == nil {
				errors.HandleError(errors.NewNotFoundError("Issue", issueA), "Failed to delete relation")
				os.Exit(1)
			}
			var relationID string
			for _, n := range find.Issue.Relations.Nodes {
				if n.Type == apiType && n.RelatedIssue.ID == toID {
					relationID = n.ID
					break
				}
			}
			if relationID == "" {
				errors.HandleError(
					errors.NewNotFoundError(
						"Relation",
						fmt.Sprintf("%s between %s and %s", relType, issueA, issueB),
					),
					"Failed to delete relation",
				)
				os.Exit(1)
			}
			del, err := client.RequestRaw(context.Background(), `
mutation IssueRelationDelete($id: String!) {
  issueRelationDelete(id: $id) { success }
}`, map[string]any{"id": relationID})
			if err != nil {
				errors.HandleError(err, "Failed to delete relation")
				os.Exit(1)
			}
			var parsed struct {
				IssueRelationDelete struct {
					Success bool `json:"success"`
				} `json:"issueRelationDelete"`
			}
			_ = json.Unmarshal(del, &parsed)
			if !parsed.IssueRelationDelete.Success {
				errors.HandleError(errors.NewCliError("Failed to delete relation"), "Failed to delete relation")
				os.Exit(1)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Deleted relation: %s %s %s\n", issueA, relType, issueB)
		},
	}
}

func newRelationListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list [issueId]",
		Short: "List relations for an issue",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			provided := ""
			if len(args) > 0 {
				provided = args[0]
			}
			resolved := mustResolveIssue(provided, "Failed to list relations")
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to list relations")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), `
query ListIssueRelations($issueId: String!) {
  issue(id: $issueId) {
    identifier
    title
    relations {
      nodes {
        id
        type
        relatedIssue { identifier title }
      }
    }
    inverseRelations {
      nodes {
        id
        type
        issue { identifier title }
      }
    }
  }
}`, map[string]any{"issueId": resolved})
			if err != nil {
				errors.HandleError(err, "Failed to list relations")
				os.Exit(1)
			}
			var parsed struct {
				Issue *struct {
					Identifier string `json:"identifier"`
					Title      string `json:"title"`
					Relations  struct {
						Nodes []struct {
							Type         string `json:"type"`
							RelatedIssue struct {
								Identifier string `json:"identifier"`
								Title      string `json:"title"`
							} `json:"relatedIssue"`
						} `json:"nodes"`
					} `json:"relations"`
					InverseRelations struct {
						Nodes []struct {
							Type  string `json:"type"`
							Issue struct {
								Identifier string `json:"identifier"`
								Title      string `json:"title"`
							} `json:"issue"`
						} `json:"nodes"`
					} `json:"inverseRelations"`
				} `json:"issue"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil || parsed.Issue == nil {
				errors.HandleError(errors.NewNotFoundError("Issue", resolved), "Failed to list relations")
				os.Exit(1)
			}
			iss := parsed.Issue
			fmt.Fprintf(cmd.OutOrStdout(), "Relations for %s: %s\n\n", iss.Identifier, iss.Title)
			if len(iss.Relations.Nodes) == 0 && len(iss.InverseRelations.Nodes) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "  No relations")
				return
			}
			for _, rel := range iss.Relations.Nodes {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s %s %s: %s\n",
					iss.Identifier, rel.Type, rel.RelatedIssue.Identifier, rel.RelatedIssue.Title)
			}
			for _, rel := range iss.InverseRelations.Nodes {
				displayType := rel.Type
				if rel.Type == "blocks" {
					displayType = "blocked-by"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  %s %s %s: %s\n",
					iss.Identifier, displayType, rel.Issue.Identifier, rel.Issue.Title)
			}
		},
	}
}

func issueUUID(client *graphql.Client, identifier string) (string, error) {
	data, err := client.RequestRaw(context.Background(), issueUUIDQuery, map[string]any{"id": identifier})
	if err != nil {
		return "", err
	}
	var parsed struct {
		Issue *struct {
			ID string `json:"id"`
		} `json:"issue"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", err
	}
	if parsed.Issue == nil {
		return "", errors.NewNotFoundError("Issue", identifier)
	}
	return parsed.Issue.ID, nil
}

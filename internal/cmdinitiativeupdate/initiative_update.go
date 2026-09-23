package cmdinitiativeupdate

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

const listInitiativeUpdatesQuery = `
query ListInitiativeUpdates($id: String!, $first: Int) {
  initiative(id: $id) {
    name
    slugId
    initiativeUpdates(first: $first) {
      nodes {
        id body health url createdAt
        user { name displayName }
      }
      pageInfo { hasNextPage endCursor }
    }
  }
}
`

const resolveInitiativeBySlug = `
query GetInitiativeBySlug($slugId: String!) {
  initiatives(filter: { slugId: { eq: $slugId } }) { nodes { id slugId } }
}
`

const resolveInitiativeByName = `
query GetInitiativeByName($name: String!) {
  initiatives(filter: { name: { eqIgnoreCase: $name } }) { nodes { id name } }
}
`

const createInitiativeUpdateMutation = `
mutation InitiativeUpdateCreate($input: InitiativeUpdateCreateInput!) {
  initiativeUpdateCreate(input: $input) {
    success
    initiativeUpdate { id url }
  }
}
`

// New returns the initiative-update command group.
func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "initiative-update",
		Short: "Manage initiative updates",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newListCommand(), newCreateCommand())
	return cmd
}

func resolveInitiativeID(client *graphql.Client, ref string) (string, error) {
	if ids.IsUUID(ref) {
		return ref, nil
	}
	for _, q := range []struct {
		query string
		key   string
		vars  map[string]any
	}{
		{resolveInitiativeBySlug, "slugId", map[string]any{"slugId": ref}},
		{resolveInitiativeByName, "name", map[string]any{"name": ref}},
	} {
		data, err := client.RequestRaw(context.Background(), q.query, q.vars)
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

func newListCommand() *cobra.Command {
	var jsonOut bool
	var limit int
	cmd := &cobra.Command{
		Use:   "list <initiativeId>",
		Short: "List status updates for an initiative",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to list initiative updates")
				os.Exit(1)
			}
			id, err := resolveInitiativeID(client, args[0])
			if err != nil {
				errors.HandleError(err, "Failed to list initiative updates")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), listInitiativeUpdatesQuery, map[string]any{
				"id": id, "first": limit,
			})
			if err != nil {
				errors.HandleError(err, "Failed to list initiative updates")
				os.Exit(1)
			}
			var parsed struct {
				Initiative *struct {
					Name              string `json:"name"`
					InitiativeUpdates struct {
						Nodes []json.RawMessage `json:"nodes"`
					} `json:"initiativeUpdates"`
				} `json:"initiative"`
			}
			_ = json.Unmarshal(data, &parsed)
			if parsed.Initiative == nil {
				errors.HandleError(errors.NewNotFoundError("Initiative", args[0]), "Failed to list initiative updates")
				os.Exit(1)
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(parsed.Initiative)
				return
			}
			for _, raw := range parsed.Initiative.InitiativeUpdates.Nodes {
				var u struct {
					Health string `json:"health"`
					Body   string `json:"body"`
					URL    string `json:"url"`
				}
				_ = json.Unmarshal(raw, &u)
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n%s\n\n", u.Health, u.URL, u.Body)
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
		Use:   "create <initiativeId>",
		Short: "Create an initiative status update",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if body == "" {
				if !prompt.IsInteractive() {
					errors.HandleError(
						errors.NewValidationError("body is required", errors.WithSuggestion("Pass --body")),
						"Failed to create initiative update",
					)
					os.Exit(1)
				}
				fmt.Fprintln(cmd.ErrOrStderr(), "Opening editor for status update content...")
				edited, err := editor.Open()
				if err != nil {
					errors.HandleError(err, "Failed to create initiative update")
					os.Exit(1)
				}
				if edited == "" {
					errors.HandleError(
						errors.NewValidationError("body is required", errors.WithSuggestion("Pass --body or provide content in the editor")),
						"Failed to create initiative update",
					)
					os.Exit(1)
				}
				body = edited
			}
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to create initiative update")
				os.Exit(1)
			}
			id, err := resolveInitiativeID(client, args[0])
			if err != nil {
				errors.HandleError(err, "Failed to create initiative update")
				os.Exit(1)
			}
			input := map[string]any{"initiativeId": id, "body": body}
			if health != "" {
				input["health"] = health
			}
			data, err := client.RequestRaw(context.Background(), createInitiativeUpdateMutation, map[string]any{"input": input})
			if err != nil {
				errors.HandleError(err, "Failed to create initiative update")
				os.Exit(1)
			}
			var parsed struct {
				InitiativeUpdateCreate struct {
					Success          bool `json:"success"`
					InitiativeUpdate *struct {
						URL string `json:"url"`
					} `json:"initiativeUpdate"`
				} `json:"initiativeUpdateCreate"`
			}
			_ = json.Unmarshal(data, &parsed)
			if !parsed.InitiativeUpdateCreate.Success || parsed.InitiativeUpdateCreate.InitiativeUpdate == nil {
				errors.HandleError(errors.NewCliError("Failed to create initiative update"), "Failed to create initiative update")
				os.Exit(1)
			}
			fmt.Fprintln(cmd.OutOrStdout(), parsed.InitiativeUpdateCreate.InitiativeUpdate.URL)
		},
	}
	cmd.Flags().StringVarP(&body, "body", "b", "", "Update body")
	cmd.Flags().StringVar(&health, "health", "", "Health")
	return cmd
}

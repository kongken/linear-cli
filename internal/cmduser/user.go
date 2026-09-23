package cmduser

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/kongken/linear-cli/internal/errors"
	"github.com/kongken/linear-cli/internal/graphql"
	"github.com/spf13/cobra"
)

const orgMembersQuery = `
query OrganizationMembers($includeDisabled: Boolean, $first: Int, $after: String) {
  users(includeDisabled: $includeDisabled, first: $first, after: $after) {
    nodes {
      id
      name
      displayName
      email
      active
      url
      admin
      guest
    }
    pageInfo { hasNextPage endCursor }
  }
}
`

// New returns the user command group.
func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage Linear users",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newListCommand())
	return cmd
}

func newListCommand() *cobra.Command {
	var (
		all     bool
		jsonOut bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List members of the workspace",
		Run: func(cmd *cobra.Command, args []string) {
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to fetch workspace members")
				os.Exit(1)
			}
			var allNodes []json.RawMessage
			var pageInfo json.RawMessage
			var after *string
			for {
				vars := map[string]any{"includeDisabled": all, "first": 50}
				if after != nil {
					vars["after"] = *after
				}
				data, err := client.RequestRaw(context.Background(), orgMembersQuery, vars)
				if err != nil {
					errors.HandleError(err, "Failed to fetch workspace members")
					os.Exit(1)
				}
				var parsed struct {
					Users struct {
						Nodes    []json.RawMessage `json:"nodes"`
						PageInfo struct {
							HasNextPage bool    `json:"hasNextPage"`
							EndCursor   *string `json:"endCursor"`
						} `json:"pageInfo"`
					} `json:"users"`
				}
				if err := json.Unmarshal(data, &parsed); err != nil {
					errors.HandleError(err, "Failed to fetch workspace members")
					os.Exit(1)
				}
				allNodes = append(allNodes, parsed.Users.Nodes...)
				pi, _ := json.Marshal(parsed.Users.PageInfo)
				pageInfo = pi
				if !parsed.Users.PageInfo.HasNextPage || parsed.Users.PageInfo.EndCursor == nil {
					break
				}
				after = parsed.Users.PageInfo.EndCursor
			}
			if !all {
				filtered := allNodes[:0]
				for _, raw := range allNodes {
					var m struct {
						Active bool `json:"active"`
					}
					_ = json.Unmarshal(raw, &m)
					if m.Active {
						filtered = append(filtered, raw)
					}
				}
				allNodes = filtered
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(map[string]any{"nodes": allNodes, "pageInfo": json.RawMessage(pageInfo)})
				return
			}
			if len(allNodes) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No members found in this workspace.")
				return
			}
			for _, raw := range allNodes {
				var m struct {
					Name        string `json:"name"`
					DisplayName string `json:"displayName"`
					Email       string `json:"email"`
				}
				_ = json.Unmarshal(raw, &m)
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t<%s>\n", m.DisplayName, m.Name, m.Email)
			}
		},
	}
	cmd.Flags().BoolVarP(&all, "all", "a", false, "Include inactive members")
	cmd.Flags().BoolVarP(&jsonOut, "json", "j", false, "Output as JSON")
	return cmd
}

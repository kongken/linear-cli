package cmdschema

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kongken/linear-cli/internal/errors"
	"github.com/kongken/linear-cli/internal/graphql"
	"github.com/spf13/cobra"
)

// New returns the schema command.
func New() *cobra.Command {
	var (
		jsonOut bool
		output  string
	)
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "Print the GraphQL schema to stdout",
		Run: func(cmd *cobra.Command, args []string) {
			var content string
			if !jsonOut {
				if sdl, err := loadLocalSDL(); err == nil {
					content = sdl
					if content != "" && content[len(content)-1] != '\n' {
						content += "\n"
					}
				}
			}
			if content == "" {
				client, err := graphql.NewClient()
				if err != nil {
					errors.HandleError(err, "Failed to fetch schema")
					os.Exit(1)
				}
				data, err := client.RequestRaw(context.Background(), introspectionQuery, nil)
				if err != nil {
					errors.HandleError(err, "Failed to fetch schema")
					os.Exit(1)
				}
				var pretty any
				_ = json.Unmarshal(data, &pretty)
				b, _ := json.MarshalIndent(pretty, "", "  ")
				content = string(b) + "\n"
				if !jsonOut {
					fmt.Fprintln(os.Stderr, "Note: local graphql/schema.graphql not found; printing introspection JSON. Pass --json to silence this note.")
				}
			}
			if output != "" {
				if err := os.WriteFile(output, []byte(content), 0o644); err != nil {
					errors.HandleError(err, "Failed to fetch schema")
					os.Exit(1)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Schema written to %s\n", output)
				return
			}
			fmt.Fprint(cmd.OutOrStdout(), content)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON introspection result instead of SDL")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Write schema to file instead of stdout")
	return cmd
}

func loadLocalSDL() (string, error) {
	candidates := []string{
		"graphql/schema.graphql",
		filepath.Join("..", "graphql", "schema.graphql"),
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "graphql", "schema.graphql"))
	}
	for _, c := range candidates {
		b, err := os.ReadFile(c)
		if err == nil {
			return string(b), nil
		}
	}
	return "", os.ErrNotExist
}

const introspectionQuery = `
query IntrospectionQuery {
  __schema {
    queryType { name }
    mutationType { name }
    subscriptionType { name }
    types { kind name description }
  }
}
`

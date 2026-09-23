package cmdapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/kongken/linear-cli/internal/errors"
	"github.com/kongken/linear-cli/internal/graphql"
	"github.com/spf13/cobra"
)

// New returns the raw GraphQL api command.
func New() *cobra.Command {
	var (
		variables    []string
		variablesJSON string
	)
	cmd := &cobra.Command{
		Use:   "api [graphqlDocument]",
		Short: "Make a raw GraphQL API request",
		Long: `Make a raw GraphQL API request.

Pass the GraphQL document as one quoted argument or on stdin.`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			doc := ""
			if len(args) > 0 {
				doc = args[0]
			} else {
				b, err := io.ReadAll(os.Stdin)
				if err != nil {
					errors.HandleError(err, "Failed to read GraphQL document")
					os.Exit(1)
				}
				doc = string(b)
			}
			doc = strings.TrimSpace(doc)
			if doc == "" {
				errors.HandleError(
					errors.NewValidationError(
						"GraphQL document is required",
						errors.WithSuggestion("Pass a query/mutation string or pipe it on stdin."),
					),
					"Failed to run api",
				)
				os.Exit(1)
			}

			vars := map[string]any{}
			if variablesJSON != "" {
				if err := json.Unmarshal([]byte(variablesJSON), &vars); err != nil {
					errors.HandleError(
						errors.NewValidationError("Invalid --variables-json: "+err.Error()),
						"Failed to run api",
					)
					os.Exit(1)
				}
			}
			for _, v := range variables {
				key, val, ok := strings.Cut(v, "=")
				if !ok {
					errors.HandleError(
						errors.NewValidationError(
							fmt.Sprintf("Invalid variable format: %s", v),
							errors.WithSuggestion("Use key=value, e.g. --variable teamId=abc"),
						),
						"Failed to run api",
					)
					os.Exit(1)
				}
				vars[key] = coerce(val)
			}

			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to run api")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), doc, vars)
			if err != nil {
				errors.HandleError(err, "Failed to run api")
				os.Exit(1)
			}
			var pretty any
			if err := json.Unmarshal(data, &pretty); err != nil {
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
				return
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			_ = enc.Encode(pretty)
		},
	}
	cmd.Flags().StringArrayVar(&variables, "variable", nil, "Variable in key=value format")
	cmd.Flags().StringVar(&variablesJSON, "variables-json", "", "JSON object of variables")
	return cmd
}

func coerce(v string) any {
	switch strings.ToLower(v) {
	case "true":
		return true
	case "false":
		return false
	case "null":
		return nil
	}
	if i, err := strconv.Atoi(v); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil {
		return f
	}
	return v
}

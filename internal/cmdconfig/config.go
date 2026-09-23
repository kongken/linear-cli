package cmdconfig

import (
	"fmt"
	"os"

	"github.com/kongken/linear-cli/internal/config"
	"github.com/kongken/linear-cli/internal/errors"
	"github.com/spf13/cobra"
)

// New returns the config command group.
func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Configure the CLI",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newGetCommand())
	return cmd
}

func newGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Print a configuration value",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			_ = config.LoadProjectConfigFromCwd()
			val, ok := config.GetOption(args[0])
			if !ok {
				errors.HandleError(
					errors.NewNotFoundError("Config key", args[0]),
					"Failed to get config",
				)
				os.Exit(1)
			}
			fmt.Fprintln(cmd.OutOrStdout(), val)
		},
	}
}

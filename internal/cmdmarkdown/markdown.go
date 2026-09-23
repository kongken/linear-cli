package cmdmarkdown

import (
	"fmt"

	"github.com/kongken/linear-cli/internal/markdownref"
	"github.com/spf13/cobra"
)

// New returns the markdown reference command.
func New() *cobra.Command {
	return &cobra.Command{
		Use:   "markdown",
		Short: "Linear-flavored Markdown: mentions and collapsible sections",
		Long:  markdownref.Reference,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(cmd.OutOrStdout(), markdownref.Reference)
		},
	}
}

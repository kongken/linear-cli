package cli

import (
	"fmt"

	"github.com/kongken/linear-cli/internal/cmdapi"
	"github.com/kongken/linear-cli/internal/cmdauth"
	"github.com/kongken/linear-cli/internal/cmdconfig"
	"github.com/kongken/linear-cli/internal/cmdcycle"
	"github.com/kongken/linear-cli/internal/cmddocument"
	"github.com/kongken/linear-cli/internal/cmdinitiative"
	"github.com/kongken/linear-cli/internal/cmdinitiativeupdate"
	"github.com/kongken/linear-cli/internal/cmdissue"
	"github.com/kongken/linear-cli/internal/cmdlabel"
	"github.com/kongken/linear-cli/internal/cmdmarkdown"
	"github.com/kongken/linear-cli/internal/cmdmilestone"
	"github.com/kongken/linear-cli/internal/cmdproject"
	"github.com/kongken/linear-cli/internal/cmdprojectupdate"
	"github.com/kongken/linear-cli/internal/cmdschema"
	"github.com/kongken/linear-cli/internal/cmdteam"
	"github.com/kongken/linear-cli/internal/cmdtemplate"
	"github.com/kongken/linear-cli/internal/cmduser"
	"github.com/kongken/linear-cli/internal/config"
	"github.com/kongken/linear-cli/internal/credentials"
	"github.com/kongken/linear-cli/internal/keyring"
	"github.com/kongken/linear-cli/internal/version"
	"github.com/spf13/cobra"
)

// NewRoot builds the linear CLI root command.
func NewRoot() *cobra.Command {
	var workspace string

	cmd := &cobra.Command{
		Use:     "linear",
		Version: version.Version,
		Short:   "Handy linear commands from the command line.",
		Long: `Handy linear commands from the command line.

Environment Variables:
  LINEAR_DEBUG=1              Show full error details including stack traces
  LINEAR_IGNORE_ENV_FILE=1    Skip loading .env files`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(cmd.OutOrStdout(), "Use --help to see available commands")
		},
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			config.SetCLIWorkspace(workspace)
			_ = config.LoadProjectConfigFromCwd()
			keyring.UseOS()
			_, _ = credentials.Load()
		},
	}

	cmd.PersistentFlags().StringVar(&workspace, "workspace", "", "Target workspace (uses credentials)")

	issueCmd := cmdissue.New()
	issueCmd.Aliases = []string{"i"}

	teamCmd := cmdteam.New()
	teamCmd.Aliases = []string{"t"}

	userCmd := cmduser.New()
	userCmd.Aliases = []string{"u"}

	projectCmd := cmdproject.New()
	projectCmd.Aliases = []string{"p"}

	projectUpdateCmd := cmdprojectupdate.New()
	projectUpdateCmd.Aliases = []string{"pu"}

	cycleCmd := cmdcycle.New()
	cycleCmd.Aliases = []string{"cy"}

	milestoneCmd := cmdmilestone.New()
	milestoneCmd.Aliases = []string{"m"}

	initiativeCmd := cmdinitiative.New()
	initiativeCmd.Aliases = []string{"init"}

	initiativeUpdateCmd := cmdinitiativeupdate.New()
	initiativeUpdateCmd.Aliases = []string{"iu"}

	labelCmd := cmdlabel.New()
	labelCmd.Aliases = []string{"l"}

	configCmd := cmdconfig.New()
	configCmd.Aliases = []string{"configure"}

	cmd.AddCommand(
		cmdauth.New(),
		issueCmd,
		teamCmd,
		userCmd,
		projectCmd,
		projectUpdateCmd,
		cycleCmd,
		milestoneCmd,
		initiativeCmd,
		initiativeUpdateCmd,
		labelCmd,
		cmdtemplate.New(),
		cmddocument.New(),
		configCmd,
		cmdschema.New(),
		cmdapi.New(),
		cmdmarkdown.New(),
	)

	return cmd
}

// Execute runs the root command.
func Execute() error {
	return NewRoot().Execute()
}

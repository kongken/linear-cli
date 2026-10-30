package cli

import (
	"fmt"

	"github.com/kongken/workctl/internal/cmdapi"
	"github.com/kongken/workctl/internal/cmdauth"
	"github.com/kongken/workctl/internal/cmdconfig"
	"github.com/kongken/workctl/internal/cmdcycle"
	"github.com/kongken/workctl/internal/cmddocument"
	"github.com/kongken/workctl/internal/cmdinitiative"
	"github.com/kongken/workctl/internal/cmdinitiativeupdate"
	"github.com/kongken/workctl/internal/cmdissue"
	"github.com/kongken/workctl/internal/cmdlabel"
	"github.com/kongken/workctl/internal/cmdmarkdown"
	"github.com/kongken/workctl/internal/cmdmilestone"
	"github.com/kongken/workctl/internal/cmdproject"
	"github.com/kongken/workctl/internal/cmdprojectupdate"
	"github.com/kongken/workctl/internal/cmdschema"
	"github.com/kongken/workctl/internal/cmdteam"
	"github.com/kongken/workctl/internal/cmdtemplate"
	"github.com/kongken/workctl/internal/cmduser"
	"github.com/kongken/workctl/internal/config"
	"github.com/kongken/workctl/internal/credentials"
	"github.com/kongken/workctl/internal/keyring"
	"github.com/kongken/workctl/internal/version"
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

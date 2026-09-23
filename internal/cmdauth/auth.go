package cmdauth

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	linearconst "github.com/kongken/linear-cli/internal/const"
	"github.com/kongken/linear-cli/internal/credentials"
	"github.com/kongken/linear-cli/internal/errors"
	"github.com/kongken/linear-cli/internal/gql"
	"github.com/kongken/linear-cli/internal/graphql"
	"github.com/spf13/cobra"
)

// New returns the auth command group.
func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage Linear authentication",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		newLoginCommand(),
		newLogoutCommand(),
		newListCommand(),
		newDefaultCommand(),
		newTokenCommand(),
		newWhoamiCommand(),
		newStatusCommand(),
		newMigrateCommand(),
	)
	return cmd
}

func newLoginCommand() *cobra.Command {
	var apiKey string
	var plaintext bool
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Add a workspace credential",
		Run: func(cmd *cobra.Command, args []string) {
			key := strings.TrimSpace(apiKey)
			if key == "" {
				errors.HandleError(
					errors.NewValidationError(
						"No API key provided",
						errors.WithSuggestion("Pass --key or create one at https://linear.app/settings/account/security"),
					),
					"Failed to login",
				)
				os.Exit(1)
			}
			key = regexp.MustCompile(`^[^a-zA-Z0-9_]+|[^a-zA-Z0-9_]+$`).ReplaceAllString(key, "")

			client := graphql.NewClientWithAPIKey(key)
			data, err := gql.ViewerWhoami(context.Background(), client)
			if err != nil {
				errors.HandleError(errors.NewAuthError("Invalid API key: "+err.Error()), "Failed to login")
				os.Exit(1)
			}
			workspace := data.Viewer.Organization.UrlKey
			already := credentials.HasWorkspace(workspace)
			if err := credentials.AddCredential(workspace, key, plaintext || credentials.UsingInlineFormat()); err != nil {
				errors.HandleError(err, "Failed to login")
				os.Exit(1)
			}
			org := data.Viewer.Organization.Name
			if already {
				fmt.Fprintf(cmd.OutOrStdout(), "Updated credentials for workspace: %s (%s)\n", org, workspace)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Logged in to workspace: %s (%s)\n", org, workspace)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  User: %s <%s>\n", data.Viewer.Name, data.Viewer.Email)
			if len(credentials.Workspaces()) == 1 {
				fmt.Fprintln(cmd.OutOrStdout(), "  Set as default workspace")
			}
			if os.Getenv("LINEAR_API_KEY") != "" {
				fmt.Fprintln(cmd.OutOrStdout())
				fmt.Fprintln(cmd.OutOrStdout(), "Warning: LINEAR_API_KEY environment variable is set.")
				fmt.Fprintln(cmd.OutOrStdout(), "It takes precedence over stored credentials.")
			}
		},
	}
	cmd.Flags().StringVarP(&apiKey, "key", "k", "", "API key (required in non-interactive Go port for now)")
	cmd.Flags().BoolVar(&plaintext, "plaintext", false, "Store API key in credentials file instead of system keyring")
	return cmd
}

func newLogoutCommand() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "logout [workspace]",
		Short: "Remove a workspace credential",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			workspaces := credentials.Workspaces()
			if len(workspaces) == 0 {
				errors.HandleError(errors.NewAuthError("No workspaces configured"), "Failed to logout")
				os.Exit(1)
			}
			workspace := ""
			if len(args) > 0 {
				workspace = args[0]
			} else if len(workspaces) == 1 {
				workspace = workspaces[0]
			} else {
				errors.HandleError(
					errors.NewValidationError(
						"workspace argument required when multiple workspaces are configured",
						errors.WithSuggestion("Pass a workspace slug or run `linear auth list`."),
					),
					"Failed to logout",
				)
				os.Exit(1)
			}
			if !credentials.HasWorkspace(workspace) {
				errors.HandleError(errors.NewNotFoundError("Workspace", workspace), "Failed to logout")
				os.Exit(1)
			}
			if !force {
				errors.HandleError(
					errors.NewValidationError(
						"confirmation required",
						errors.WithSuggestion("Re-run with --force to remove credentials."),
					),
					"Failed to logout",
				)
				os.Exit(1)
			}
			if err := credentials.RemoveCredential(workspace); err != nil {
				errors.HandleError(err, "Failed to logout")
				os.Exit(1)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed credentials for workspace: %s\n", workspace)
			if def := credentials.DefaultWorkspace(); def != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  Default workspace is now: %s\n", def)
			}
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")
	return cmd
}

func newListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured workspaces",
		Run: func(cmd *cobra.Command, args []string) {
			workspaces := credentials.Workspaces()
			if len(workspaces) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No workspaces configured")
				fmt.Fprintln(cmd.OutOrStdout(), "Run `linear auth login` to add a workspace")
				return
			}
			def := credentials.DefaultWorkspace()
			for _, ws := range workspaces {
				prefix := "  "
				if ws == def {
					prefix = "* "
				}
				key, ok := credentials.GetAPIKey(ws)
				if !ok {
					fmt.Fprintf(cmd.OutOrStdout(), "%s%s  missing credentials\n", prefix, ws)
					continue
				}
				client := graphql.NewClientWithAPIKey(key)
				data, err := gql.ViewerWhoami(context.Background(), client)
				if err != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "%s%s  invalid credentials\n", prefix, ws)
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s%s  %s  %s <%s>\n",
					prefix, ws, data.Viewer.Organization.Name, data.Viewer.Name, data.Viewer.Email)
			}
		},
	}
}

func newDefaultCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "default [workspace]",
		Short: "Set the default workspace",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			workspaces := credentials.Workspaces()
			if len(workspaces) == 0 {
				errors.HandleError(
					errors.NewAuthError("No workspaces configured", errors.WithSuggestion("Run `linear auth login` to add a workspace")),
					"Failed to set default workspace",
				)
				os.Exit(1)
			}
			if len(workspaces) == 1 {
				fmt.Fprintf(cmd.OutOrStdout(), "Only one workspace configured: %s\n", workspaces[0])
				return
			}
			if len(args) == 0 {
				errors.HandleError(
					errors.NewValidationError(
						"workspace argument required",
						errors.WithSuggestion(fmt.Sprintf("Available workspaces: %s", strings.Join(workspaces, ", "))),
					),
					"Failed to set default workspace",
				)
				os.Exit(1)
			}
			workspace := args[0]
			if !credentials.HasWorkspace(workspace) {
				errors.HandleError(
					errors.NewNotFoundError("Workspace", workspace, errors.WithSuggestion(
						fmt.Sprintf("Available workspaces: %s", strings.Join(workspaces, ", ")),
					)),
					"Failed to set default workspace",
				)
				os.Exit(1)
			}
			if workspace == credentials.DefaultWorkspace() {
				fmt.Fprintf(cmd.OutOrStdout(), "%q is already the default workspace\n", workspace)
				return
			}
			if err := credentials.SetDefaultWorkspace(workspace); err != nil {
				errors.HandleError(err, "Failed to set default workspace")
				os.Exit(1)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Default workspace set to: %s\n", workspace)
		},
	}
}

func newTokenCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "token",
		Short: "Print the configured API token",
		Run: func(cmd *cobra.Command, args []string) {
			key, err := graphql.ResolvedAPIKey()
			if err != nil {
				errors.HandleError(err, "Failed to get API token")
				os.Exit(1)
			}
			fmt.Fprintln(cmd.OutOrStdout(), key)
		},
	}
}

func newWhoamiCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Print information about the authenticated user",
		Run: func(cmd *cobra.Command, args []string) {
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to get user info")
				os.Exit(1)
			}
			data, err := gql.ViewerWhoami(context.Background(), client)
			if err != nil {
				errors.HandleError(err, "Failed to get user info")
				os.Exit(1)
			}
			v := data.Viewer
			org := v.Organization
			fmt.Fprintf(cmd.OutOrStdout(), "Workspace: %s\n", org.Name)
			fmt.Fprintf(cmd.OutOrStdout(), "  Slug: %s\n", org.UrlKey)
			fmt.Fprintf(cmd.OutOrStdout(), "  URL: %s/%s\n", linearconst.WebBaseURL, org.UrlKey)
			fmt.Fprintf(cmd.OutOrStdout(), "User: %s\n", v.Name)
			if v.DisplayName != v.Name {
				fmt.Fprintf(cmd.OutOrStdout(), "  Display name: %s\n", v.DisplayName)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  Email: %s\n", v.Email)
			if v.Admin {
				fmt.Fprintln(cmd.OutOrStdout(), "  Role: admin")
			} else if v.Guest {
				fmt.Fprintln(cmd.OutOrStdout(), "  Role: guest")
			}
		},
	}
}

func newStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Print information about the authenticated user",
		Run: func(cmd *cobra.Command, args []string) {
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to get auth status")
				os.Exit(1)
			}
			data, err := gql.ViewerWhoami(context.Background(), client)
			if err != nil {
				errors.HandleError(err, "Failed to get auth status")
				os.Exit(1)
			}
			v := data.Viewer
			org := v.Organization
			fmt.Fprintf(cmd.OutOrStdout(), "Workspace: %s\n", org.Name)
			fmt.Fprintf(cmd.OutOrStdout(), "  Slug: %s\n", org.UrlKey)
			fmt.Fprintf(cmd.OutOrStdout(), "  URL: %s/%s\n", linearconst.WebBaseURL, org.UrlKey)
			fmt.Fprintf(cmd.OutOrStdout(), "User: %s\n", v.Name)
			if v.DisplayName != v.Name {
				fmt.Fprintf(cmd.OutOrStdout(), "  Display name: %s\n", v.DisplayName)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  Email: %s\n", v.Email)
			if v.Admin {
				fmt.Fprintln(cmd.OutOrStdout(), "  Role: admin")
			} else if v.Guest {
				fmt.Fprintln(cmd.OutOrStdout(), "  Role: guest")
			}
			inline := credentials.UsingInlineFormat()
			storage := "system keyring"
			if inline {
				storage = "plaintext file"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Credential storage: %s\n", storage)
			if inline {
				fmt.Fprintln(cmd.OutOrStdout(), "  Run `linear auth migrate` to migrate to the system keyring.")
			}
		},
	}
}

func newMigrateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "migrate",
		Short: "Migrate plaintext credentials to the system keyring",
		Run: func(cmd *cobra.Command, args []string) {
			if !credentials.UsingInlineFormat() {
				fmt.Fprintln(cmd.OutOrStdout(), "Credentials are already using the system keyring.")
				return
			}
			migrated, err := credentials.MigrateToKeyring()
			if err != nil {
				errors.HandleError(
					errors.NewCliError(
						"No system keyring found. Cannot migrate credentials.",
						errors.WithSuggestion("Install libsecret, or set LINEAR_API_KEY instead."),
						errors.WithCause(err),
					),
					"Failed to migrate credentials",
				)
				os.Exit(1)
			}
			if len(migrated) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No credentials to migrate.")
				return
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Migrated %d workspace(s) to system keyring:\n", len(migrated))
			for _, ws := range migrated {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", ws)
			}
		},
	}
}

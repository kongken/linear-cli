package cmdplane

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kongken/workctl/internal/plane"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func (s *settings) authCommand() *cobra.Command {
	root := &cobra.Command{Use: "auth", Short: "Manage Plane personal access tokens"}
	var tokenStdin, plaintext bool
	login := &cobra.Command{Use: "login", Short: "Validate a PAT and save a named Plane profile", Args: exact(0), RunE: s.run("log in to Plane", func(cmd *cobra.Command, _ []string) error {
		if s.context.NoProfile || s.context.Profile == "" {
			return invalid("login requires --profile <name> and cannot use --no-profile")
		}
		cfg, err := s.options.Resolver.LoginContext(s.context)
		if err != nil {
			return invalid(err.Error())
		}
		token := s.env("PLANE_API_KEY")
		if tokenStdin {
			if token != "" {
				return invalid("--token-stdin and PLANE_API_KEY are mutually exclusive")
			}
			data, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), 65537))
			if err != nil {
				return err
			}
			if len(data) > 65536 {
				return invalid("token exceeds maximum size")
			}
			token = strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
		} else if token == "" {
			if cmd.InOrStdin() != os.Stdin || !term.IsTerminal(int(os.Stdin.Fd())) {
				return invalid("set PLANE_API_KEY or pass --token-stdin")
			}
			if _, err := fmt.Fprint(cmd.ErrOrStderr(), "Plane personal access token: "); err != nil {
				return err
			}
			data, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Fprintln(cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			token = string(data)
		}
		c, err := plane.NewClient(cfg, token, plane.ClientOptions{HTTP: s.options.HTTP})
		if err != nil {
			return invalid(err.Error())
		}
		if _, err := c.CurrentUser(cmd.Context()); err != nil {
			return err
		}
		if _, err := c.List(cmd.Context(), "projects", "", nil); err != nil {
			return err
		}
		if err := s.manager().Save(cfg, token, plaintext); err != nil {
			return err
		}
		if err := s.options.Resolver.SaveProfile(cfg.ProfileName, cfg.Profile); err != nil {
			return fmt.Errorf("token saved but unable to save profile metadata: %w", err)
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Logged in to Plane profile %s (%s, %s).\n", cfg.ProfileName, cfg.BaseURL, cfg.Workspace)
		return err
	})}
	login.Flags().BoolVar(&tokenStdin, "token-stdin", false, "Read the PAT from stdin")
	login.Flags().BoolVar(&plaintext, "plaintext", false, "Explicitly store the PAT in a private plaintext file instead of the system keyring")
	root.AddCommand(login)
	for _, name := range []string{"status", "whoami"} {
		root.AddCommand(&cobra.Command{Use: name, Short: "Validate the current Plane identity and workspace", Args: exact(0), RunE: s.run("check Plane authentication", func(cmd *cobra.Command, _ []string) error {
			c, err := s.client()
			if err != nil {
				return err
			}
			raw, err := c.CurrentUser(cmd.Context())
			if err != nil {
				return err
			}
			if _, err := c.List(cmd.Context(), "projects", "", nil); err != nil {
				return err
			}
			return writeJSON(cmd.OutOrStdout(), raw)
		})})
	}
	root.AddCommand(&cobra.Command{Use: "logout", Short: "Remove stored credentials for the selected Plane profile", Args: exact(0), RunE: s.run("log out of Plane", func(cmd *cobra.Command, _ []string) error {
		cfg, err := s.options.Resolver.Resolve(s.context)
		if err != nil {
			return invalid(err.Error())
		}
		if err := s.manager().Logout(cfg); err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), "Stored Plane credential removed. Environment tokens are unchanged.")
		return err
	})})
	return root
}

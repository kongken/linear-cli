// Package cmdplane contains the Plane command tree; it never loads Linear state.
package cmdplane

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	clierrors "github.com/kongken/workctl/internal/errors"
	"github.com/kongken/workctl/internal/keyring"
	"github.com/kongken/workctl/internal/plane"
	"github.com/kongken/workctl/internal/planeauth"
	"github.com/kongken/workctl/internal/planeconfig"
	"github.com/spf13/cobra"
)

type Options struct {
	Resolver planeconfig.Resolver
	HTTP     *http.Client
	Keyring  keyring.Store
}
type settings struct {
	options Options
	context planeconfig.Options
	timeout time.Duration
}

func New() *cobra.Command { return NewWithOptions(Options{}) }
func NewWithOptions(options Options) *cobra.Command {
	s := &settings{options: options}
	root := &cobra.Command{Use: "plane", Short: "Manage Plane workspaces and work items", SilenceErrors: true, SilenceUsage: true}
	f := root.PersistentFlags()
	f.StringVar(&s.context.Profile, "profile", "", "Plane profile")
	f.BoolVar(&s.context.NoProfile, "no-profile", false, "Use explicit flags/environment without a stored profile")
	f.StringVar(&s.context.BaseURL, "base-url", "", "Plane instance base URL (Cloud by default)")
	f.StringVar(&s.context.Workspace, "workspace", "", "Plane workspace slug")
	f.StringVar(&s.context.APIVersion, "api-version", "", "Plane API version: v1 or v2 (Cloud defaults to v2)")
	f.StringVar(&s.context.ProjectID, "project", "", "Plane project UUID, identifier or unique name")
	f.BoolVar(&s.context.AllowHTTP, "allow-http", false, "Explicitly allow an HTTP development instance")
	f.DurationVar(&s.timeout, "timeout", 2*time.Minute, "Overall command deadline")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return invalid(err.Error()) })
	root.AddCommand(s.authCommand(), s.profileCommand(), s.configCommand(), s.apiCommand(), s.issueCommand())
	for _, r := range []struct{ name, resource string }{{"project", "projects"}, {"state", "states"}, {"label", "labels"}, {"user", "members"}, {"cycle", "cycles"}, {"module", "modules"}, {"estimate", "estimates"}} {
		root.AddCommand(s.directoryCommand(r.name, r.resource))
	}
	var registerGroups func(*cobra.Command)
	registerGroups = func(cmd *cobra.Command) {
		for _, child := range cmd.Commands() {
			registerGroups(child)
		}
		if cmd.HasSubCommands() && cmd.RunE == nil && cmd.Run == nil {
			cmd.RunE = func(cmd *cobra.Command, args []string) error {
				if len(args) > 0 {
					return invalid("unknown command " + args[0] + "; use --help to list available commands")
				}
				return cmd.Help()
			}
		}
	}
	registerGroups(root)

	return root
}
func invalid(message string) error { return clierrors.NewValidationError(message) }
func exact(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := cobra.ExactArgs(n)(cmd, args); err != nil {
			return invalid(err.Error())
		}
		return nil
	}
}
func (s *settings) run(action string, fn func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if s.timeout <= 0 {
			return invalid("--timeout must be positive")
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), s.timeout)
		defer cancel()
		cmd.SetContext(ctx)
		if err := fn(cmd, args); err != nil {
			var api *plane.APIError
			suggestion := ""
			if errors.As(err, &api) && api.Status == 401 {
				suggestion = "Check PLANE_API_KEY or run workctl plane auth login for this profile."
			}
			if errors.As(err, &api) && api.Status == 403 {
				suggestion = "Check your Plane workspace/project permissions and token scopes."
			}
			return clierrors.NewCliError("Failed to "+action+": "+err.Error(), clierrors.WithCause(err), clierrors.WithSuggestion(suggestion))
		}
		return nil
	}
}
func (s *settings) env(key string) string {
	if s.options.Resolver.Getenv != nil {
		return s.options.Resolver.Getenv(key)
	}
	return os.Getenv(key)
}
func (s *settings) manager() planeauth.Manager {
	return planeauth.Manager{Resolver: s.options.Resolver, Store: s.options.Keyring}
}
func (s *settings) client() (*plane.Client, error) {
	cfg, err := s.options.Resolver.Resolve(s.context)
	if err != nil {
		return nil, invalid(err.Error())
	}
	token := s.env("PLANE_API_KEY")
	if token == "" {
		if cfg.ProfileName == "" {
			return nil, clierrors.NewAuthError("No Plane token configured", clierrors.WithSuggestion("Set PLANE_API_KEY or run workctl plane auth login."))
		}
		token, err = s.manager().Token(cfg)
		if err != nil {
			return nil, clierrors.NewAuthError(err.Error(), clierrors.WithSuggestion("Set PLANE_API_KEY or run workctl plane auth login."))
		}
	}
	return plane.NewClient(cfg, token, plane.ClientOptions{HTTP: s.options.HTTP})
}
func writeJSON(w io.Writer, raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var out strings.Builder
	// Indent preserves all provider fields, numbers and nesting.
	var buf json.RawMessage = raw
	data, err := json.MarshalIndent(buf, "", "  ")
	if err != nil {
		return err
	}
	out.Write(data)
	out.WriteByte('\n')
	_, err = io.WriteString(w, out.String())
	return err
}
func writeRows(w io.Writer, rows []json.RawMessage) error {
	out := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	for _, raw := range rows {
		var row struct {
			ID          string `json:"id"`
			Identifier  string `json:"identifier"`
			Name        string `json:"name"`
			Email       string `json:"email"`
			DisplayName string `json:"display_name"`
			Comment     string `json:"comment_stripped"`
			Member      struct {
				DisplayName string `json:"display_name"`
				Email       string `json:"email"`
			} `json:"member"`
		}
		if err := json.Unmarshal(raw, &row); err != nil {
			return fmt.Errorf("invalid resource response")
		}
		key := row.Identifier
		if key == "" {
			key = row.ID
		}
		name := row.Name
		if name == "" {
			name = row.DisplayName
		}
		if name == "" {
			name = row.Member.DisplayName
		}
		if name == "" {
			name = row.Email
		}
		if name == "" {
			name = row.Member.Email
		}
		if name == "" {
			name = row.Comment
		}
		name = strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(name)
		if _, err := fmt.Fprintf(out, "%s\t%s\n", key, name); err != nil {
			return err
		}
	}
	return out.Flush()
}
func (s *settings) configCommand() *cobra.Command {
	root := &cobra.Command{Use: "config", Short: "Inspect Plane configuration"}
	root.AddCommand(&cobra.Command{Use: "view", Short: "Show resolved context without credentials", Args: exact(0), RunE: s.run("read Plane configuration", func(cmd *cobra.Command, _ []string) error {
		cfg, err := s.options.Resolver.Resolve(s.context)
		if err != nil {
			return invalid(err.Error())
		}
		source := "profile credential store"
		if s.env("PLANE_API_KEY") != "" {
			source = "PLANE_API_KEY"
		} else if cfg.ProfileName == "" {
			source = "none"
		}
		raw, err := json.Marshal(struct {
			Context     planeconfig.Context `json:"context"`
			TokenSource string              `json:"token_source"`
		}{cfg, source})
		if err != nil {
			return err
		}
		return writeJSON(cmd.OutOrStdout(), raw)
	})})
	return root
}
func (s *settings) profileCommand() *cobra.Command {
	root := &cobra.Command{Use: "profile", Short: "Manage Plane profile selection"}
	root.AddCommand(&cobra.Command{Use: "list", Short: "List configured Plane profiles", Args: exact(0), RunE: s.run("list Plane profiles", func(cmd *cobra.Command, _ []string) error {
		cfg, err := s.options.Resolver.Load()
		if err != nil {
			return invalid(err.Error())
		}
		var names []string
		for name := range cfg.Plane.Profiles {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			p := cfg.Plane.Profiles[name]
			p, err = planeconfig.Normalize(p)
			if err != nil {
				return invalid("invalid Plane profile " + name + ": " + err.Error())
			}
			mark := " "
			if name == cfg.Plane.DefaultProfile {
				mark = "*"
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %s\t%s\t%s\t%s\n", mark, name, p.BaseURL, p.Workspace, p.APIVersion); err != nil {
				return err
			}
		}
		return nil
	})}, &cobra.Command{Use: "use <name>", Short: "Set the default Plane profile", Args: exact(1), RunE: s.run("select Plane profile", func(cmd *cobra.Command, args []string) error {
		if err := s.options.Resolver.SetDefault(args[0]); err != nil {
			return invalid(err.Error())
		}
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "Default Plane profile: %s\n", args[0])
		return err
	})})
	return root
}

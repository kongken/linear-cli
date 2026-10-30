package cmdplane

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/kongken/workctl/internal/plane"
	"github.com/spf13/cobra"
)

func readNativeInput(cmd *cobra.Command, file string) (json.RawMessage, error) {
	if file == "" {
		return nil, invalid("--input is required; provide a native JSON object for the selected API version")
	}
	var data []byte
	var err error
	if file == "-" {
		data, err = io.ReadAll(io.LimitReader(cmd.InOrStdin(), 16*1024*1024+1))
	} else {
		data, err = os.ReadFile(file)
	}
	if err != nil {
		return nil, err
	}
	if len(data) > 16*1024*1024 {
		return nil, invalid("request body exceeds 16 MiB")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil || len(object) == 0 {
		return nil, invalid("--input must contain a nonempty JSON object")
	}
	return json.RawMessage(data), nil
}
func (s *settings) resourceAdmin(root *cobra.Command, resource string) {
	for _, action := range []string{"create", "update", "delete"} {
		action := action
		var input string
		var yes bool
		use := action
		n := 0
		if action != "create" {
			use += " <id>"
			n = 1
		}
		cmd := &cobra.Command{Use: use, Short: strings.ToUpper(action[:1]) + action[1:] + " a resource using the selected API's native JSON", Args: exact(n), RunE: s.run(action+" Plane resource", func(cmd *cobra.Command, args []string) error {
			if action == "delete" && !yes {
				return invalid("pass --yes to confirm resource deletion")
			}
			var body json.RawMessage
			var err error
			if action != "delete" {
				body, err = readNativeInput(cmd, input)
				if err != nil {
					return err
				}
			}
			c, err := s.client()
			if err != nil {
				return err
			}
			project := ""
			if resource != "projects" {
				project, err = resolveProject(cmd.Context(), c, "", true)
				if err != nil {
					return err
				}
			}
			id := ""
			if len(args) > 0 {
				if resource == "projects" {
					id, err = resolveProject(cmd.Context(), c, args[0], true)
				} else {
					id, err = resolveRelation(cmd.Context(), c, resource, project, args[0])
				}
				if err != nil {
					return err
				}
			}
			method := http.MethodPost
			if action == "update" {
				method = http.MethodPatch
			}
			if action == "delete" {
				method = http.MethodDelete
			}
			raw, err := c.MutateResource(cmd.Context(), resource, project, id, method, body)
			if err != nil {
				return err
			}
			return writeJSON(cmd.OutOrStdout(), raw)
		})}
		if action == "delete" {
			cmd.Flags().BoolVar(&yes, "yes", false, "Confirm resource deletion")
		} else {
			cmd.Flags().StringVar(&input, "input", "", "Native JSON object file for this API version, or - for stdin")
		}
		root.AddCommand(cmd)
	}
	if resource == "cycles" || resource == "modules" {
		for _, add := range []bool{true, false} {
			add := add
			action := "add-issue"
			if !add {
				action = "remove-issue"
			}
			root.AddCommand(&cobra.Command{Use: action + " <container> <issue>", Short: "Change one work-item association", Args: exact(2), RunE: s.run("change Plane association", func(cmd *cobra.Command, args []string) error {
				c, err := s.client()
				if err != nil {
					return err
				}
				project, err := resolveProject(cmd.Context(), c, "", true)
				if err != nil {
					return err
				}
				id, err := resolveRelation(cmd.Context(), c, resource, project, args[0])
				if err != nil {
					return err
				}
				ref, err := c.ResolveIssue(cmd.Context(), project, args[1])
				if err != nil {
					return err
				}
				raw, err := c.Membership(cmd.Context(), resource, project, id, ref.ID, add)
				if err != nil {
					return err
				}
				return writeJSON(cmd.OutOrStdout(), raw)
			})})
		}
	}
}
func (s *settings) issueActions(root *cobra.Command) {
	for _, action := range []string{"delete", "archive", "unarchive"} {
		action := action
		var yes bool
		cmd := &cobra.Command{Use: action + " <issue>", Short: strings.ToUpper(action[:1]) + action[1:] + " a work item", Args: exact(1), RunE: s.run(action+" Plane work item", func(cmd *cobra.Command, args []string) error {
			if !yes {
				return invalid("pass --yes to confirm work-item " + action)
			}
			c, err := s.client()
			if err != nil {
				return err
			}
			if action != "delete" && c.Context.APIVersion != "v2" {
				return invalid("archive/unarchive commands require the v2 API; v1 can use explicit native API operations")
			}
			project, err := resolveProject(cmd.Context(), c, "", action == "unarchive")
			if err != nil {
				return err
			}
			var ref plane.IssueRef
			if action == "unarchive" {
				// Archived objects are absent from normal detail reads, so address the action directly.
				if _, e := uuid.Parse(args[0]); e != nil && !strings.Contains(args[0], "-") {
					return invalid("unarchive requires a UUID or full identifier and --project")
				}
				ref = plane.IssueRef{ProjectID: project, ID: args[0]}
			} else {
				ref, err = c.ResolveIssue(cmd.Context(), project, args[0])
				if err != nil {
					return err
				}
			}
			raw, err := c.IssueAction(cmd.Context(), ref, action)
			if err != nil {
				return err
			}
			return writeJSON(cmd.OutOrStdout(), raw)
		})}
		cmd.Flags().BoolVar(&yes, "yes", false, "Confirm this work-item action")
		root.AddCommand(cmd)
	}
	root.AddCommand(s.childCollection("link", "links"), s.childCollection("attachment", "attachments"))
}
func (s *settings) childCollection(name, resource string) *cobra.Command {
	root := &cobra.Command{Use: name, Short: "Manage work-item " + name + " resources"}
	flags := new(listFlags)
	list := &cobra.Command{Use: "list <issue>", Short: "List work-item " + name + " resources", Args: exact(1), RunE: s.run("list Plane "+name+" resources", func(cmd *cobra.Command, args []string) error {
		c, err := s.client()
		if err != nil {
			return err
		}
		q, err := flags.query(cmd, c)
		if err != nil {
			return err
		}
		project, err := resolveProject(cmd.Context(), c, "", false)
		if err != nil {
			return err
		}
		ref, err := c.ResolveIssue(cmd.Context(), project, args[0])
		if err != nil {
			return err
		}
		path, err := c.SubresourcePath(ref, resource)
		if err != nil {
			return err
		}
		return flags.output(cmd, c, path, q)
	})}
	flags.bind(list)
	root.AddCommand(list)
	for _, action := range []string{"update", "delete"} {
		if resource == "attachments" && action == "update" {
			continue
		}
		action := action
		var input string
		var yes bool
		cmd := &cobra.Command{Use: action + " <issue> <id>", Short: strings.ToUpper(action[:1]) + action[1:] + " a work-item " + name, Args: exact(2), RunE: s.run(action+" Plane "+name, func(cmd *cobra.Command, args []string) error {
			if action == "delete" && !yes {
				return invalid("pass --yes to confirm deletion")
			}
			var body json.RawMessage
			var err error
			if action == "update" {
				body, err = readNativeInput(cmd, input)
				if err != nil {
					return err
				}
			}
			c, err := s.client()
			if err != nil {
				return err
			}
			project, err := resolveProject(cmd.Context(), c, "", false)
			if err != nil {
				return err
			}
			ref, err := c.ResolveIssue(cmd.Context(), project, args[0])
			if err != nil {
				return err
			}
			method := http.MethodPatch
			if action == "delete" {
				method = http.MethodDelete
			}
			raw, err := c.MutateSubresource(cmd.Context(), ref, resource, args[1], method, body)
			if err != nil {
				return err
			}
			return writeJSON(cmd.OutOrStdout(), raw)
		})}
		if action == "delete" {
			cmd.Flags().BoolVar(&yes, "yes", false, "Confirm deletion")
		} else {
			cmd.Flags().StringVar(&input, "input", "", "Native JSON object file, or - for stdin")
		}
		root.AddCommand(cmd)
	}
	if resource == "links" {
		var title string
		add := &cobra.Command{Use: "add <issue> <url>", Short: "Add a link to a work item", Args: exact(2), RunE: s.run("add Plane link", func(cmd *cobra.Command, args []string) error {
			target, e := url.Parse(args[1])
			if e != nil || target.Host == "" || (target.Scheme != "https" && target.Scheme != "http") {
				return invalid("link URL must be an absolute HTTP or HTTPS URL")
			}
			c, err := s.client()
			if err != nil {
				return err
			}
			project, err := resolveProject(cmd.Context(), c, "", false)
			if err != nil {
				return err
			}
			ref, err := c.ResolveIssue(cmd.Context(), project, args[0])
			if err != nil {
				return err
			}
			body, err := json.Marshal(struct {
				URL   string `json:"url"`
				Title string `json:"title,omitempty"`
			}{args[1], title})
			if err != nil {
				return err
			}
			raw, err := c.MutateSubresource(cmd.Context(), ref, "links", "", http.MethodPost, body)
			if err != nil {
				return err
			}
			return writeJSON(cmd.OutOrStdout(), raw)
		})}
		add.Flags().StringVar(&title, "title", "", "Link title")
		root.AddCommand(add)
	}
	return root
}
func (s *settings) commentChanges(root *cobra.Command) {
	var body, file, format string
	update := &cobra.Command{Use: "update <issue> <comment>", Short: "Replace a comment body", Args: exact(2), RunE: s.run("update Plane comment", func(cmd *cobra.Command, args []string) error {
		if cmd.Flags().Changed("body") && file != "" {
			return invalid("--body and --body-file are mutually exclusive")
		}
		html, err := content(body, file, format)
		if err != nil {
			return err
		}
		if strings.TrimSpace(html) == "" {
			return invalid("comment content is required")
		}
		c, err := s.client()
		if err != nil {
			return err
		}
		project, err := resolveProject(cmd.Context(), c, "", false)
		if err != nil {
			return err
		}
		ref, err := c.ResolveIssue(cmd.Context(), project, args[0])
		if err != nil {
			return err
		}
		payload, err := json.Marshal(struct {
			HTML string `json:"comment_html"`
		}{html})
		if err != nil {
			return err
		}
		raw, err := c.MutateSubresource(cmd.Context(), ref, "comments", args[1], http.MethodPatch, payload)
		if err != nil {
			return err
		}
		return writeJSON(cmd.OutOrStdout(), raw)
	})}
	update.Flags().StringVar(&body, "body", "", "Replacement comment body")
	update.Flags().StringVar(&file, "body-file", "", "Read replacement body from a file")
	update.Flags().StringVar(&format, "body-format", "markdown", "Content format: markdown or html")
	root.AddCommand(update)
	var yes bool
	del := &cobra.Command{Use: "delete <issue> <comment>", Short: "Delete a comment", Args: exact(2), RunE: s.run("delete Plane comment", func(cmd *cobra.Command, args []string) error {
		if !yes {
			return invalid("pass --yes to confirm comment deletion")
		}
		c, err := s.client()
		if err != nil {
			return err
		}
		project, err := resolveProject(cmd.Context(), c, "", false)
		if err != nil {
			return err
		}
		ref, err := c.ResolveIssue(cmd.Context(), project, args[0])
		if err != nil {
			return err
		}
		raw, err := c.MutateSubresource(cmd.Context(), ref, "comments", args[1], http.MethodDelete, nil)
		if err != nil {
			return err
		}
		return writeJSON(cmd.OutOrStdout(), raw)
	})}
	del.Flags().BoolVar(&yes, "yes", false, "Confirm comment deletion")
	root.AddCommand(del)
}

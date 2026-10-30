package cmdplane

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/kongken/workctl/internal/plane"
	"github.com/spf13/cobra"
	htmlparser "golang.org/x/net/html"
)

func plainHTML(source string) string {
	doc, err := htmlparser.Parse(strings.NewReader(source))
	if err != nil {
		return source
	}
	var text strings.Builder
	var visit func(*htmlparser.Node)
	visit = func(node *htmlparser.Node) {
		if node.Type == htmlparser.ElementNode && (node.Data == "script" || node.Data == "style") {
			return
		}
		if node.Type == htmlparser.TextNode {
			text.WriteString(node.Data)
		}
		if node.Type == htmlparser.ElementNode && node.Data == "br" {
			text.WriteByte('\n')
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
		if node.Type == htmlparser.ElementNode {
			switch node.Data {
			case "p", "div", "li", "h1", "h2", "h3", "pre", "blockquote":
				text.WriteByte('\n')
			}
		}
	}
	visit(doc)
	return strings.TrimSpace(text.String())
}
func (s *settings) attachmentCommand() *cobra.Command {
	var mimeType string
	cmd := &cobra.Command{Use: "attach <issue> <file>", Short: "Upload and confirm a file attachment", Args: exact(2), RunE: s.run("upload Plane attachment", func(cmd *cobra.Command, args []string) error {
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
		raw, err := c.UploadAttachment(cmd.Context(), ref, args[1], mimeType)
		if err != nil {
			return err
		}
		return writeJSON(cmd.OutOrStdout(), raw)
	})}
	cmd.Flags().StringVar(&mimeType, "content-type", "", "Override the file MIME type")
	return cmd
}
func (s *settings) summaryCommands(root *cobra.Command) {
	for _, field := range []string{"id", "title", "describe", "url"} {
		field := field
		label := map[string]string{"id": "identifier", "title": "title", "describe": "description", "url": "web URL"}[field]
		root.AddCommand(&cobra.Command{Use: field + " <issue>", Short: "Print the work-item " + label, Args: exact(1), RunE: s.run("read Plane work-item "+label, func(cmd *cobra.Command, args []string) error {
			c, err := s.client()
			if err != nil {
				return err
			}
			project, err := resolveProject(cmd.Context(), c, "", false)
			if err != nil {
				return err
			}
			raw, err := c.GetIssue(cmd.Context(), project, args[0])
			if err != nil {
				return err
			}
			var item struct {
				ID         string `json:"id"`
				Identifier string `json:"identifier"`
				Name       string `json:"name"`
				HTML       string `json:"description_html"`
				URL        string `json:"url"`
			}
			if err := json.Unmarshal(raw, &item); err != nil {
				return fmt.Errorf("invalid work-item response")
			}
			value := item.Identifier
			if value == "" {
				value = item.ID
			}
			switch field {
			case "title":
				value = item.Name
			case "describe":
				value = plainHTML(item.HTML)
			case "url":
				value = item.URL
				if value == "" {
					return invalid("this Plane API response does not expose a work-item web URL; use issue view --json instead")
				}
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), value)
			return err
		})})
	}
}
func runGit(ctx context.Context, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s failed: %s", args[0], strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}
func (s *settings) startCommand() *cobra.Command {
	var branch, from, state string
	cmd := &cobra.Command{Use: "start <issue>", Short: "Select an issue branch and move the work item to a started state", Args: exact(1), RunE: s.run("start Plane work item", func(cmd *cobra.Command, args []string) error {
		status, err := runGit(cmd.Context(), "status", "--porcelain")
		if err != nil {
			return err
		}
		if status != "" {
			return invalid("commit or stash local changes before starting work")
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
		stateID := ""
		if state != "" {
			stateID, err = resolveRelation(cmd.Context(), c, "states", ref.ProjectID, state)
			if err != nil {
				return err
			}
		} else {
			rows, e := c.Directory(cmd.Context(), "states", ref.ProjectID)
			if e != nil {
				return e
			}
			for _, raw := range rows {
				var row struct {
					ID    string `json:"id"`
					Group string `json:"group"`
				}
				if e := json.Unmarshal(raw, &row); e != nil {
					return e
				}
				if row.Group == "started" {
					if stateID != "" {
						return invalid("multiple started states exist; choose one with --state")
					}
					stateID = row.ID
				}
			}
			if stateID == "" {
				return invalid("no started state is configured; choose a state with --state")
			}
		}
		selected := branch
		if selected == "" {
			var item struct {
				Identifier string `json:"identifier"`
			}
			if err := json.Unmarshal(ref.Raw, &item); err != nil {
				return err
			}
			selected = strings.ToLower(item.Identifier)
			if selected == "" {
				return invalid("work item has no identifier; provide --branch")
			}
		}
		if _, err := runGit(cmd.Context(), "check-ref-format", "--branch", selected); err != nil {
			return invalid("choose a valid Git branch name")
		}
		if from != "" {
			from, err = runGit(cmd.Context(), "rev-parse", "--verify", "--end-of-options", from+"^{commit}")
			if err != nil {
				return err
			}
		}
		probe := exec.CommandContext(cmd.Context(), "git", "show-ref", "--verify", "--quiet", "refs/heads/"+selected)
		if probe.Run() == nil {
			if from != "" {
				return invalid("--from-ref cannot be used with an existing branch")
			}
			if _, err := runGit(cmd.Context(), "switch", selected); err != nil {
				return err
			}
		} else {
			arguments := []string{"switch", "-c", selected}
			if from != "" {
				arguments = append(arguments, from)
			}
			if _, err := runGit(cmd.Context(), arguments...); err != nil {
				return err
			}
		}
		if _, err := c.UpdateIssue(cmd.Context(), ref, plane.IssueInput{State: plane.Set(&stateID)}); err != nil {
			return fmt.Errorf("Git branch %s is selected, but the Plane state update failed: %w", selected, err)
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Started %s on branch %s.\n", args[0], selected)
		return err
	})}
	cmd.Flags().StringVar(&branch, "branch", "", "Choose a Git branch instead of the issue identifier")
	cmd.Flags().StringVar(&from, "from-ref", "", "Create a new branch from this Git ref")
	cmd.Flags().StringVar(&state, "state", "", "Started state UUID or unique name")
	return cmd
}

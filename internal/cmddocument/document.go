package cmddocument

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/kongken/linear-cli/internal/ids"
	"github.com/kongken/linear-cli/internal/display"
	"github.com/kongken/linear-cli/internal/editor"
	"github.com/kongken/linear-cli/internal/errors"
	"github.com/kongken/linear-cli/internal/graphql"
	"github.com/kongken/linear-cli/internal/prompt"
	"github.com/spf13/cobra"
)

const listDocumentsQuery = `
query ListDocuments($filter: DocumentFilter, $first: Int) {
  documents(filter: $filter, first: $first) {
    nodes {
      id title slugId url updatedAt
      project { name slugId }
      issue { identifier title }
      initiative { name slugId }
      team { name key }
      creator { name }
    }
    pageInfo { hasNextPage endCursor }
  }
}
`

const getDocumentQuery = `
query GetDocument($id: String!) {
  document(id: $id) {
    id title slugId content url createdAt updatedAt
    creator { name email }
    project { name slugId }
    issue { identifier title }
    initiative { name slugId }
    team { name key }
  }
}
`

// New returns the document command group.
func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "document",
		Short: "Manage Linear documents",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		newListCommand(),
		newViewCommand(),
		newCreateCommand(),
		newUpdateCommand(),
		newDeleteCommand(),
		newCommentCommand(),
	)
	return cmd
}

func newListCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List documents",
		Run: func(cmd *cobra.Command, args []string) {
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to list documents")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), listDocumentsQuery, map[string]any{"first": 50})
			if err != nil {
				errors.HandleError(err, "Failed to list documents")
				os.Exit(1)
			}
			var parsed struct {
				Documents struct {
					Nodes    []json.RawMessage `json:"nodes"`
					PageInfo json.RawMessage   `json:"pageInfo"`
				} `json:"documents"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil {
				errors.HandleError(err, "Failed to list documents")
				os.Exit(1)
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(map[string]any{
					"nodes":    parsed.Documents.Nodes,
					"pageInfo": parsed.Documents.PageInfo,
				})
				return
			}
			for _, raw := range parsed.Documents.Nodes {
				var d struct {
					Title string `json:"title"`
					URL   string `json:"url"`
				}
				_ = json.Unmarshal(raw, &d)
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", d.Title, d.URL)
			}
		},
	}
	cmd.Flags().BoolVarP(&jsonOut, "json", "j", false, "Output as JSON")
	return cmd
}

func newViewCommand() *cobra.Command {
	var raw, jsonOut bool
	cmd := &cobra.Command{
		Use:     "view <id>",
		Aliases: []string{"v"},
		Short:   "View a document's content",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to view document")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), getDocumentQuery, map[string]any{"id": args[0]})
			if err != nil {
				errors.HandleError(err, "Failed to view document")
				os.Exit(1)
			}
			var parsed struct {
				Document *struct {
					Title   string  `json:"title"`
					SlugID  string  `json:"slugId"`
					Content *string `json:"content"`
					URL     string  `json:"url"`
					Creator *struct {
						Name string `json:"name"`
					} `json:"creator"`
					Project *struct {
						Name string `json:"name"`
					} `json:"project"`
					Issue *struct {
						Identifier string `json:"identifier"`
						Title      string `json:"title"`
					} `json:"issue"`
				} `json:"document"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil || parsed.Document == nil {
				errors.HandleError(errors.NewNotFoundError("Document", args[0]), "Failed to view document")
				os.Exit(1)
			}
			doc := parsed.Document
			if jsonOut {
				var pretty any
				_ = json.Unmarshal(data, &pretty)
				if m, ok := pretty.(map[string]any); ok {
					pretty = m["document"]
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(pretty)
				return
			}
			if raw {
				if doc.Content != nil {
					fmt.Fprintln(cmd.OutOrStdout(), *doc.Content)
				}
				return
			}
			var b strings.Builder
			fmt.Fprintf(&b, "# %s\n\n", doc.Title)
			fmt.Fprintf(&b, "**Slug:** %s\n", doc.SlugID)
			fmt.Fprintf(&b, "**URL:** %s\n", doc.URL)
			if doc.Creator != nil {
				fmt.Fprintf(&b, "**Creator:** %s\n", doc.Creator.Name)
			}
			if doc.Project != nil {
				fmt.Fprintf(&b, "**Project:** %s\n", doc.Project.Name)
			}
			if doc.Issue != nil {
				fmt.Fprintf(&b, "**Issue:** %s - %s\n", doc.Issue.Identifier, doc.Issue.Title)
			}
			if doc.Content != nil && *doc.Content != "" {
				fmt.Fprintf(&b, "\n---\n\n%s\n", *doc.Content)
			}
			rendered, err := display.Markdown(b.String())
			if err != nil {
				fmt.Fprint(cmd.OutOrStdout(), b.String())
				return
			}
			fmt.Fprint(cmd.OutOrStdout(), rendered)
		},
	}
	cmd.Flags().BoolVar(&raw, "raw", false, "Output raw markdown without rendering")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	return cmd
}

func newCreateCommand() *cobra.Command {
	var title, content, project string
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"c"},
		Short:   "Create a document",
		Run: func(cmd *cobra.Command, args []string) {
			if title == "" {
				errors.HandleError(
					errors.NewValidationError("Document title is required", errors.WithSuggestion("Use --title or -t")),
					"Failed to create document",
				)
				os.Exit(1)
			}
			if content == "" && prompt.IsInteractive() {
				fmt.Fprintln(cmd.ErrOrStderr(), "Opening editor for document content...")
				edited, err := editor.Open()
				if err != nil {
					errors.HandleError(err, "Failed to create document")
					os.Exit(1)
				}
				content = edited
			}
			input := map[string]any{"title": title}
			if content != "" {
				input["content"] = content
			}
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to create document")
				os.Exit(1)
			}
			if project != "" {
				pid, err := resolveProjectID(client, project)
				if err != nil {
					errors.HandleError(err, "Failed to create document")
					os.Exit(1)
				}
				input["projectId"] = pid
			}
			data, err := client.RequestRaw(context.Background(), `
mutation CreateDocument($input: DocumentCreateInput!) {
  documentCreate(input: $input) {
    success
    document { id title slugId url }
  }
}`, map[string]any{"input": input})
			if err != nil {
				errors.HandleError(err, "Failed to create document")
				os.Exit(1)
			}
			var parsed struct {
				DocumentCreate struct {
					Success  bool `json:"success"`
					Document *struct {
						Title string `json:"title"`
						URL   string `json:"url"`
					} `json:"document"`
				} `json:"documentCreate"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil || !parsed.DocumentCreate.Success || parsed.DocumentCreate.Document == nil {
				errors.HandleError(errors.NewCliError("Failed to create document"), "Failed to create document")
				os.Exit(1)
			}
			d := parsed.DocumentCreate.Document
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Created document: %s\n", d.Title)
			if d.URL != "" {
				fmt.Fprintln(cmd.OutOrStdout(), d.URL)
			}
		},
	}
	cmd.Flags().StringVarP(&title, "title", "t", "", "Document title")
	cmd.Flags().StringVarP(&content, "content", "c", "", "Markdown content")
	cmd.Flags().StringVar(&project, "project", "", "Attach to project")
	return cmd
}

func newUpdateCommand() *cobra.Command {
	var title, content string
	var edit bool
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a document",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if title == "" && content == "" && !edit {
				errors.HandleError(
					errors.NewValidationError("At least one update option required", errors.WithSuggestion("Use --title, --content, or --edit")),
					"Failed to update document",
				)
				os.Exit(1)
			}
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to update document")
				os.Exit(1)
			}
			input := map[string]any{}
			if title != "" {
				input["title"] = title
			}
			if edit {
				data, err := client.RequestRaw(context.Background(), getDocumentQuery, map[string]any{"id": args[0]})
				if err != nil {
					errors.HandleError(err, "Failed to update document")
					os.Exit(1)
				}
				var current struct {
					Document *struct {
						Title   string  `json:"title"`
						Content *string `json:"content"`
					} `json:"document"`
				}
				if err := json.Unmarshal(data, &current); err != nil || current.Document == nil {
					errors.HandleError(errors.NewNotFoundError("Document", args[0]), "Failed to update document")
					os.Exit(1)
				}
				initial := ""
				if current.Document.Content != nil {
					initial = *current.Document.Content
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "Opening %s in editor...\n", current.Document.Title)
				edited, err := editor.OpenWithContent(initial)
				if err != nil {
					errors.HandleError(err, "Failed to update document")
					os.Exit(1)
				}
				input["content"] = edited
			} else if content != "" {
				input["content"] = content
			}
			data, err := client.RequestRaw(context.Background(), `
mutation UpdateDocument($id: String!, $input: DocumentUpdateInput!) {
  documentUpdate(id: $id, input: $input) {
    success
    document { id title url }
  }
}`, map[string]any{"id": args[0], "input": input})
			if err != nil {
				errors.HandleError(err, "Failed to update document")
				os.Exit(1)
			}
			var parsed struct {
				DocumentUpdate struct {
					Success  bool `json:"success"`
					Document *struct {
						Title string `json:"title"`
						URL   string `json:"url"`
					} `json:"document"`
				} `json:"documentUpdate"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil || !parsed.DocumentUpdate.Success {
				errors.HandleError(errors.NewCliError("Failed to update document"), "Failed to update document")
				os.Exit(1)
			}
			if parsed.DocumentUpdate.Document != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "✓ Updated document: %s\n", parsed.DocumentUpdate.Document.Title)
			}
		},
	}
	cmd.Flags().StringVarP(&title, "title", "t", "", "Document title")
	cmd.Flags().StringVarP(&content, "content", "c", "", "Markdown content")
	cmd.Flags().BoolVarP(&edit, "edit", "e", false, "Open current content in $EDITOR for editing")
	return cmd
}

func newDeleteCommand() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a document",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ok, err := prompt.Confirm(fmt.Sprintf("Are you sure you want to delete document %s?", args[0]), force)
			if err != nil {
				errors.HandleError(err, "Failed to delete document")
				os.Exit(1)
			}
			if !ok {
				fmt.Fprintln(cmd.OutOrStdout(), "Deletion canceled")
				return
			}
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to delete document")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), `
mutation DeleteDocument($id: String!) {
  documentDelete(id: $id) { success }
}`, map[string]any{"id": args[0]})
			if err != nil {
				errors.HandleError(err, "Failed to delete document")
				os.Exit(1)
			}
			var parsed struct {
				DocumentDelete struct {
					Success bool `json:"success"`
				} `json:"documentDelete"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil || !parsed.DocumentDelete.Success {
				errors.HandleError(errors.NewCliError("Failed to delete document"), "Failed to delete document")
				os.Exit(1)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Deleted document: %s\n", args[0])
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation")
	return cmd
}

func newCommentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "comment",
		Short: "Manage document comments",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newDocCommentListCommand(), newDocCommentAddCommand())
	return cmd
}

func newDocCommentListCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list <id>",
		Short: "List document comments",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to list document comments")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), `
query DocumentComments($id: String!) {
  document(id: $id) {
    comments(first: 50) {
      nodes { id body createdAt user { name displayName } }
      pageInfo { hasNextPage endCursor }
    }
  }
}`, map[string]any{"id": args[0]})
			if err != nil {
				errors.HandleError(err, "Failed to list document comments")
				os.Exit(1)
			}
			var parsed struct {
				Document *struct {
					Comments struct {
						Nodes    []json.RawMessage `json:"nodes"`
						PageInfo json.RawMessage   `json:"pageInfo"`
					} `json:"comments"`
				} `json:"document"`
			}
			_ = json.Unmarshal(data, &parsed)
			if parsed.Document == nil {
				errors.HandleError(errors.NewNotFoundError("Document", args[0]), "Failed to list document comments")
				os.Exit(1)
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(map[string]any{
					"nodes":    parsed.Document.Comments.Nodes,
					"pageInfo": parsed.Document.Comments.PageInfo,
				})
				return
			}
			for _, raw := range parsed.Document.Comments.Nodes {
				var c struct {
					Body string `json:"body"`
					User *struct {
						DisplayName string `json:"displayName"`
						Name        string `json:"name"`
					} `json:"user"`
				}
				_ = json.Unmarshal(raw, &c)
				who := ""
				if c.User != nil {
					who = c.User.DisplayName
					if who == "" {
						who = c.User.Name
					}
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", who, c.Body)
			}
		},
	}
	cmd.Flags().BoolVarP(&jsonOut, "json", "j", false, "Output as JSON")
	return cmd
}

func newDocCommentAddCommand() *cobra.Command {
	var body string
	cmd := &cobra.Command{
		Use:   "add <id>",
		Short: "Add a comment to a document",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if strings.TrimSpace(body) == "" {
				errors.HandleError(
					errors.NewValidationError("comment body is required", errors.WithSuggestion("Pass --body")),
					"Failed to add document comment",
				)
				os.Exit(1)
			}
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to add document comment")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), `
mutation CreateDocumentComment($input: CommentCreateInput!) {
  commentCreate(input: $input) { success comment { id } }
}`, map[string]any{"input": map[string]any{"documentId": args[0], "body": body}})
			if err != nil {
				errors.HandleError(err, "Failed to add document comment")
				os.Exit(1)
			}
			var parsed struct {
				CommentCreate struct {
					Success bool `json:"success"`
				} `json:"commentCreate"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil || !parsed.CommentCreate.Success {
				errors.HandleError(errors.NewCliError("Failed to add document comment"), "Failed to add document comment")
				os.Exit(1)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "✓ Comment added")
		},
	}
	cmd.Flags().StringVarP(&body, "body", "b", "", "Comment body")
	return cmd
}

func resolveProjectID(client *graphql.Client, ref string) (string, error) {
	if ids.IsUUID(ref) {
		return ref, nil
	}
	data, err := client.RequestRaw(context.Background(), `
query ResolveProjectForDoc($name: String!) {
  projects(filter: { name: { eqIgnoreCase: $name } }, first: 5) {
    nodes { id }
  }
}`, map[string]any{"name": ref})
	if err != nil {
		return "", err
	}
	var parsed struct {
		Projects struct {
			Nodes []struct {
				ID string `json:"id"`
			} `json:"nodes"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Projects.Nodes) == 0 {
		return "", errors.NewNotFoundError("Project", ref)
	}
	return parsed.Projects.Nodes[0].ID, nil
}

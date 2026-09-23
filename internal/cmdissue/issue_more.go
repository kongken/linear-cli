package cmdissue

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/kongken/linear-cli/internal/bulk"
	"github.com/kongken/linear-cli/internal/errors"
	"github.com/kongken/linear-cli/internal/gql"
	"github.com/kongken/linear-cli/internal/graphql"
	"github.com/kongken/linear-cli/internal/linear"
	"github.com/kongken/linear-cli/internal/prompt"
	"github.com/kongken/linear-cli/internal/upload"
	"github.com/spf13/cobra"
)

const issueCommentsQuery = `
query GetIssueComments($id: String!, $after: String) {
  issue(id: $id) {
    comments(first: 50, after: $after, orderBy: createdAt) {
      nodes {
        id
        body
        createdAt
        url
        user { name displayName }
        resolvingUser { id }
        parent { id }
      }
      pageInfo { hasNextPage endCursor }
    }
  }
}
`

const commentCreateMutation = `
mutation AddComment($input: CommentCreateInput!) {
  commentCreate(input: $input) {
    success
    comment { id url }
  }
}
`

const deleteMutation = `
mutation DeleteIssue($id: String!) {
  issueDelete(id: $id) {
    success
    entity { identifier title }
  }
}
`

const attachmentLinkMutation = `
mutation AttachmentLinkURL($issueId: String!, $url: String!, $title: String) {
  attachmentLinkURL(issueId: $issueId, url: $url, title: $title) {
    success
    attachment { id title url }
  }
}
`

const issueUUIDQuery = `
query GetIssueId($id: String!) {
  issue(id: $id) { id }
}
`

func newCommentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "comment",
		Short: "Manage issue comments",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newCommentListCommand(), newCommentAddCommand(), newCommentUpdateCommand(), newCommentDeleteCommand())
	return cmd
}

func newCommentListCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list [issueId]",
		Short: "List comments for an issue",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			provided := ""
			if len(args) > 0 {
				provided = args[0]
			}
			resolved := mustResolveIssue(provided, "Failed to list comments")
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to list comments")
				os.Exit(1)
			}
			var all []json.RawMessage
			var after *string
			for {
				vars := map[string]any{"id": resolved}
				if after != nil {
					vars["after"] = *after
				}
				data, err := client.RequestRaw(context.Background(), issueCommentsQuery, vars)
				if err != nil {
					errors.HandleError(err, "Failed to list comments")
					os.Exit(1)
				}
				var parsed struct {
					Issue *struct {
						Comments struct {
							Nodes    []json.RawMessage `json:"nodes"`
							PageInfo struct {
								HasNextPage bool    `json:"hasNextPage"`
								EndCursor   *string `json:"endCursor"`
							} `json:"pageInfo"`
						} `json:"comments"`
					} `json:"issue"`
				}
				if err := json.Unmarshal(data, &parsed); err != nil {
					errors.HandleError(err, "Failed to list comments")
					os.Exit(1)
				}
				if parsed.Issue == nil {
					errors.HandleError(errors.NewNotFoundError("Issue", resolved), "Failed to list comments")
					os.Exit(1)
				}
				all = append(all, parsed.Issue.Comments.Nodes...)
				if !parsed.Issue.Comments.PageInfo.HasNextPage || parsed.Issue.Comments.PageInfo.EndCursor == nil {
					break
				}
				after = parsed.Issue.Comments.PageInfo.EndCursor
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(all)
				return
			}
			if len(all) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No comments found for this issue")
				return
			}
			for _, raw := range all {
				var c struct {
					Body string `json:"body"`
					User *struct {
						Name string `json:"name"`
					} `json:"user"`
					CreatedAt string `json:"createdAt"`
				}
				_ = json.Unmarshal(raw, &c)
				who := "unknown"
				if c.User != nil {
					who = c.User.Name
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %s\n%s\n\n", who, c.CreatedAt, c.Body)
			}
		},
	}
	cmd.Flags().BoolVarP(&jsonOut, "json", "j", false, "Output as JSON")
	return cmd
}

func newCommentAddCommand() *cobra.Command {
	var body, bodyFile, replyTo string
	cmd := &cobra.Command{
		Use:   "add [issueId]",
		Short: "Add a comment to an issue",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if body != "" && bodyFile != "" {
				errors.HandleError(
					errors.NewValidationError("Cannot specify both --body and --body-file"),
					"Failed to add comment",
				)
				os.Exit(1)
			}
			text := body
			if bodyFile != "" {
				b, err := os.ReadFile(bodyFile)
				if err != nil {
					errors.HandleError(err, "Failed to add comment")
					os.Exit(1)
				}
				text = string(b)
			}
			if strings.TrimSpace(text) == "" {
				t, err := prompt.Text("Comment body", "")
				if err != nil {
					errors.HandleError(err, "Failed to add comment")
					os.Exit(1)
				}
				text = t
			}
			if strings.TrimSpace(text) == "" {
				errors.HandleError(
					errors.NewValidationError(
						"comment body is required",
						errors.WithSuggestion("Pass --body or --body-file, or run in a terminal for interactive mode."),
					),
					"Failed to add comment",
				)
				os.Exit(1)
			}
			provided := ""
			if len(args) > 0 {
				provided = args[0]
			}
			resolved := mustResolveIssue(provided, "Failed to add comment")
			input := map[string]any{"issueId": resolved, "body": text}
			if replyTo != "" {
				input["parentId"] = replyTo
			}
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to add comment")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), commentCreateMutation, map[string]any{"input": input})
			if err != nil {
				errors.HandleError(err, "Failed to add comment")
				os.Exit(1)
			}
			var parsed struct {
				CommentCreate struct {
					Success bool `json:"success"`
					Comment *struct {
						ID  string `json:"id"`
						URL string `json:"url"`
					} `json:"comment"`
				} `json:"commentCreate"`
			}
			_ = json.Unmarshal(data, &parsed)
			if !parsed.CommentCreate.Success || parsed.CommentCreate.Comment == nil {
				errors.HandleError(errors.NewCliError("Failed to create comment"), "Failed to add comment")
				os.Exit(1)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Comment added: %s\n", parsed.CommentCreate.Comment.URL)
		},
	}
	cmd.Flags().StringVarP(&body, "body", "b", "", "Comment body")
	cmd.Flags().StringVar(&bodyFile, "body-file", "", "Read body from file")
	cmd.Flags().StringVarP(&replyTo, "reply-to", "p", "", "Parent comment id")
	return cmd
}

func newCommentUpdateCommand() *cobra.Command {
	var body, bodyFile string
	cmd := &cobra.Command{
		Use:   "update <commentId>",
		Short: "Update an existing comment",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if body != "" && bodyFile != "" {
				errors.HandleError(
					errors.NewValidationError("Cannot specify both --body and --body-file"),
					"Failed to update comment",
				)
				os.Exit(1)
			}
			text := body
			if bodyFile != "" {
				b, err := os.ReadFile(bodyFile)
				if err != nil {
					errors.HandleError(
						errors.NewValidationError(fmt.Sprintf("Failed to read body file: %s", bodyFile)),
						"Failed to update comment",
					)
					os.Exit(1)
				}
				text = string(b)
			}
			if strings.TrimSpace(text) == "" {
				t, err := prompt.Text("Comment body", "")
				if err != nil {
					errors.HandleError(err, "Failed to update comment")
					os.Exit(1)
				}
				text = t
			}
			if strings.TrimSpace(text) == "" {
				errors.HandleError(
					errors.NewValidationError(
						"comment body is required",
						errors.WithSuggestion("Pass --body or --body-file, or run in a terminal for interactive edit."),
					),
					"Failed to update comment",
				)
				os.Exit(1)
			}
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to update comment")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), `
mutation UpdateComment($id: String!, $input: CommentUpdateInput!) {
  commentUpdate(id: $id, input: $input) {
    success
    comment { id body url }
  }
}`, map[string]any{"id": args[0], "input": map[string]any{"body": text}})
			if err != nil {
				errors.HandleError(err, "Failed to update comment")
				os.Exit(1)
			}
			var parsed struct {
				CommentUpdate struct {
					Success bool `json:"success"`
					Comment *struct {
						URL string `json:"url"`
					} `json:"comment"`
				} `json:"commentUpdate"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil || !parsed.CommentUpdate.Success {
				errors.HandleError(errors.NewCliError("Failed to update comment"), "Failed to update comment")
				os.Exit(1)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "✓ Comment updated")
			if parsed.CommentUpdate.Comment != nil && parsed.CommentUpdate.Comment.URL != "" {
				fmt.Fprintln(cmd.OutOrStdout(), parsed.CommentUpdate.Comment.URL)
			}
		},
	}
	cmd.Flags().StringVarP(&body, "body", "b", "", "New comment body")
	cmd.Flags().StringVar(&bodyFile, "body-file", "", "Read body from file")
	return cmd
}

func newCommentDeleteCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <commentId>",
		Short: "Delete a comment",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to delete comment")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), `
mutation DeleteComment($id: String!) {
  commentDelete(id: $id) { success }
}`, map[string]any{"id": args[0]})
			if err != nil {
				errors.HandleError(err, "Failed to delete comment")
				os.Exit(1)
			}
			var parsed struct {
				CommentDelete struct {
					Success bool `json:"success"`
				} `json:"commentDelete"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil || !parsed.CommentDelete.Success {
				errors.HandleError(errors.NewCliError("Failed to delete comment"), "Failed to delete comment")
				os.Exit(1)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "✓ Comment deleted")
		},
	}
}

func newArchiveCommand() *cobra.Command {
	var confirm bool
	var bulkIDs []string
	var bulkFile string
	var bulkStdin bool
	cmd := &cobra.Command{
		Use:   "archive [issueId]",
		Short: "Archive an issue",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			provided := ""
			if len(args) > 0 {
				provided = args[0]
			}
			src := bulk.Sources{Bulk: bulkIDs, File: bulkFile, Stdin: bulkStdin, Positional: provided}
			if bulk.IsMode(src) {
				ids, err := bulk.Collect(src)
				if err != nil {
					errors.HandleError(err, "Failed to archive issue")
					os.Exit(1)
				}
				ok, err := prompt.ConfirmWithFlag(
					fmt.Sprintf("Are you sure you want to archive %d issues?", len(ids)),
					confirm,
					"--confirm",
				)
				if err != nil {
					errors.HandleError(err, "Failed to archive issue")
					os.Exit(1)
				}
				if !ok {
					fmt.Fprintln(cmd.OutOrStdout(), "Archive canceled")
					return
				}
				client, err := graphql.NewClient()
				if err != nil {
					errors.HandleError(err, "Failed to archive issue")
					os.Exit(1)
				}
				results := make([]bulk.Result, 0, len(ids))
				for _, id := range ids {
					resolved, err := linear.IssueIdentifier(id)
					if err != nil || resolved == "" {
						msg := "could not resolve"
						if err != nil {
							msg = err.Error()
						}
						results = append(results, bulk.Result{ID: id, Error: msg})
						continue
					}
					resp, err := gql.ArchiveIssue(context.Background(), client, resolved)
					if err != nil {
						results = append(results, bulk.Result{ID: resolved, Error: err.Error()})
						continue
					}
					if !resp.IssueArchive.Success {
						results = append(results, bulk.Result{ID: resolved, Error: "archive unsuccessful"})
						continue
					}
					results = append(results, bulk.Result{ID: resolved, Success: true})
				}
				bulk.PrintSummary(cmd.OutOrStdout(), results)
				return
			}
			resolved := mustResolveIssue(provided, "Failed to archive issue")
			ok, err := prompt.ConfirmWithFlag(
				fmt.Sprintf("Are you sure you want to archive issue %s?", resolved),
				confirm,
				"--confirm",
			)
			if err != nil {
				errors.HandleError(err, "Failed to archive issue")
				os.Exit(1)
			}
			if !ok {
				fmt.Fprintln(cmd.OutOrStdout(), "Archive canceled")
				return
			}
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to archive issue")
				os.Exit(1)
			}
			resp, err := gql.ArchiveIssue(context.Background(), client, resolved)
			if err != nil {
				errors.HandleError(err, "Failed to archive issue")
				os.Exit(1)
			}
			if !resp.IssueArchive.Success {
				errors.HandleError(errors.NewCliError("Linear reported the archive as unsuccessful"), "Failed to archive issue")
				os.Exit(1)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Successfully archived issue: %s\n", resolved)
		},
	}
	cmd.Flags().BoolVarP(&confirm, "confirm", "y", false, "Skip confirmation prompt")
	cmd.Flags().StringArrayVar(&bulkIDs, "bulk", nil, "Archive multiple issues by identifier")
	cmd.Flags().StringVar(&bulkFile, "bulk-file", "", "Read issue identifiers from a file")
	cmd.Flags().BoolVar(&bulkStdin, "bulk-stdin", false, "Read issue identifiers from stdin")
	return cmd
}

func newDeleteCommand() *cobra.Command {
	var confirm bool
	var bulkIDs []string
	var bulkFile string
	var bulkStdin bool
	cmd := &cobra.Command{
		Use:     "delete [issueId]",
		Aliases: []string{"d"},
		Short:   "Delete an issue",
		Args:    cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			provided := ""
			if len(args) > 0 {
				provided = args[0]
			}
			src := bulk.Sources{Bulk: bulkIDs, File: bulkFile, Stdin: bulkStdin, Positional: provided}
			if bulk.IsMode(src) {
				ids, err := bulk.Collect(src)
				if err != nil {
					errors.HandleError(err, "Failed to delete issue")
					os.Exit(1)
				}
				ok, err := prompt.ConfirmWithFlag(
					fmt.Sprintf("Are you sure you want to delete %d issues?", len(ids)),
					confirm,
					"--confirm",
				)
				if err != nil {
					errors.HandleError(err, "Failed to delete issue")
					os.Exit(1)
				}
				if !ok {
					fmt.Fprintln(cmd.OutOrStdout(), "Deletion canceled")
					return
				}
				client, err := graphql.NewClient()
				if err != nil {
					errors.HandleError(err, "Failed to delete issue")
					os.Exit(1)
				}
				results := make([]bulk.Result, 0, len(ids))
				for _, id := range ids {
					resolved, err := linear.IssueIdentifier(id)
					if err != nil || resolved == "" {
						msg := "could not resolve"
						if err != nil {
							msg = err.Error()
						}
						results = append(results, bulk.Result{ID: id, Error: msg})
						continue
					}
					data, err := client.RequestRaw(context.Background(), deleteMutation, map[string]any{"id": resolved})
					if err != nil {
						results = append(results, bulk.Result{ID: resolved, Error: err.Error()})
						continue
					}
					var parsed struct {
						IssueDelete struct {
							Success bool `json:"success"`
						} `json:"issueDelete"`
					}
					_ = json.Unmarshal(data, &parsed)
					if !parsed.IssueDelete.Success {
						results = append(results, bulk.Result{ID: resolved, Error: "delete unsuccessful"})
						continue
					}
					results = append(results, bulk.Result{ID: resolved, Success: true})
				}
				bulk.PrintSummary(cmd.OutOrStdout(), results)
				return
			}
			if provided == "" {
				errors.HandleError(
					errors.NewValidationError(
						"Could not determine issue ID",
						errors.WithSuggestion("Please provide an issue ID like 'ENG-123'."),
					),
					"Failed to delete issue",
				)
				os.Exit(1)
			}
			resolved := mustResolveIssue(provided, "Failed to delete issue")
			ok, err := prompt.ConfirmWithFlag(
				fmt.Sprintf("Are you sure you want to delete issue %s?", resolved),
				confirm,
				"--confirm",
			)
			if err != nil {
				errors.HandleError(err, "Failed to delete issue")
				os.Exit(1)
			}
			if !ok {
				fmt.Fprintln(cmd.OutOrStdout(), "Deletion canceled")
				return
			}
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to delete issue")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), deleteMutation, map[string]any{"id": resolved})
			if err != nil {
				errors.HandleError(err, "Failed to delete issue")
				os.Exit(1)
			}
			var parsed struct {
				IssueDelete struct {
					Success bool `json:"success"`
					Entity  *struct {
						Identifier string `json:"identifier"`
						Title      string `json:"title"`
					} `json:"entity"`
				} `json:"issueDelete"`
			}
			_ = json.Unmarshal(data, &parsed)
			if !parsed.IssueDelete.Success {
				errors.HandleError(errors.NewCliError("Failed to delete issue"), "Failed to delete issue")
				os.Exit(1)
			}
			id, title := resolved, ""
			if parsed.IssueDelete.Entity != nil {
				id = parsed.IssueDelete.Entity.Identifier
				title = parsed.IssueDelete.Entity.Title
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Successfully deleted issue: %s: %s\n", id, title)
		},
	}
	cmd.Flags().BoolVarP(&confirm, "confirm", "y", false, "Skip confirmation prompt")
	cmd.Flags().StringArrayVar(&bulkIDs, "bulk", nil, "Delete multiple issues by identifier")
	cmd.Flags().StringVar(&bulkFile, "bulk-file", "", "Read issue identifiers from a file")
	cmd.Flags().BoolVar(&bulkStdin, "bulk-stdin", false, "Read issue identifiers from stdin")
	return cmd
}

func newDescribeCommand() *cobra.Command {
	var references bool
	cmd := &cobra.Command{
		Use:   "describe [issueId]",
		Short: "Print the issue title and Linear-issue trailer",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			provided := ""
			if len(args) > 0 {
				provided = args[0]
			}
			resolved := mustResolveIssue(provided, "Failed to get issue description")
			issue, err := linear.FetchIssueTitleURL(context.Background(), resolved)
			if err != nil {
				errors.HandleError(err, "Failed to get issue description")
				os.Exit(1)
			}
			magic := "Fixes"
			if references {
				magic = "References"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n\nLinear-issue: %s %s\nLinear-issue-url: %s\n",
				resolved, issue.Title, magic, resolved, issue.URL)
		},
	}
	cmd.Flags().BoolVarP(&references, "references", "r", false, "Use 'References' instead of 'Fixes'")
	return cmd
}

func newLinkCommand() *cobra.Command {
	var title string
	cmd := &cobra.Command{
		Use:   "link <urlOrIssueId> [url]",
		Short: "Link a URL to an issue",
		Args:  cobra.RangeArgs(1, 2),
		Run: func(cmd *cobra.Command, args []string) {
			var issueInput, linkURL string
			if len(args) == 2 {
				issueInput, linkURL = args[0], args[1]
			} else if strings.HasPrefix(args[0], "http://") || strings.HasPrefix(args[0], "https://") {
				linkURL = args[0]
			} else {
				errors.HandleError(
					errors.NewValidationError(
						"URL required",
						errors.WithSuggestion("Pass a URL, or issue id then URL."),
					),
					"Failed to link URL",
				)
				os.Exit(1)
			}
			resolved := mustResolveIssue(issueInput, "Failed to link URL")
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to link URL")
				os.Exit(1)
			}
			idData, err := client.RequestRaw(context.Background(), issueUUIDQuery, map[string]any{"id": resolved})
			if err != nil {
				errors.HandleError(err, "Failed to link URL")
				os.Exit(1)
			}
			var idParsed struct {
				Issue *struct {
					ID string `json:"id"`
				} `json:"issue"`
			}
			_ = json.Unmarshal(idData, &idParsed)
			if idParsed.Issue == nil {
				errors.HandleError(errors.NewNotFoundError("Issue", resolved), "Failed to link URL")
				os.Exit(1)
			}
			vars := map[string]any{"issueId": idParsed.Issue.ID, "url": linkURL}
			if title != "" {
				vars["title"] = title
			}
			data, err := client.RequestRaw(context.Background(), attachmentLinkMutation, vars)
			if err != nil {
				errors.HandleError(err, "Failed to link URL")
				os.Exit(1)
			}
			var parsed struct {
				AttachmentLinkURL struct {
					Success    bool `json:"success"`
					Attachment *struct {
						Title string `json:"title"`
					} `json:"attachment"`
				} `json:"attachmentLinkURL"`
			}
			_ = json.Unmarshal(data, &parsed)
			if !parsed.AttachmentLinkURL.Success || parsed.AttachmentLinkURL.Attachment == nil {
				errors.HandleError(errors.NewCliError("Failed to link URL to issue"), "Failed to link URL")
				os.Exit(1)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Linked to %s: %s\n", resolved, parsed.AttachmentLinkURL.Attachment.Title)
		},
	}
	cmd.Flags().StringVarP(&title, "title", "t", "", "Custom title for the link")
	return cmd
}

func mustResolveIssue(provided, failContext string) string {
	resolved, err := linear.IssueIdentifier(provided)
	if err != nil {
		errors.HandleError(err, failContext)
		os.Exit(1)
	}
	if resolved == "" {
		errors.HandleError(
			errors.NewValidationError(
				"Could not determine issue ID",
				errors.WithSuggestion("Please provide an issue ID like 'ENG-123'."),
			),
			failContext,
		)
		os.Exit(1)
	}
	return resolved
}

func resolveIssueUUID(client *graphql.Client, identifier string) (string, error) {
	data, err := client.RequestRaw(context.Background(), issueUUIDQuery, map[string]any{"id": identifier})
	if err != nil {
		return "", err
	}
	var parsed struct {
		Issue *struct {
			ID string `json:"id"`
		} `json:"issue"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", err
	}
	if parsed.Issue == nil {
		return "", errors.NewNotFoundError("Issue", identifier)
	}
	return parsed.Issue.ID, nil
}

func newAttachCommand() *cobra.Command {
	var title, comment string
	var makePublic bool
	cmd := &cobra.Command{
		Use:   "attach <issueId> <filepath>",
		Short: "Attach a file to an issue",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			resolved := mustResolveIssue(args[0], "Failed to attach file")
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to attach file")
				os.Exit(1)
			}
			issueUUID, err := resolveIssueUUID(client, resolved)
			if err != nil {
				errors.HandleError(err, "Failed to attach file")
				os.Exit(1)
			}
			result, err := upload.File(context.Background(), args[1], makePublic)
			if err != nil {
				errors.HandleError(err, "Failed to attach file")
				os.Exit(1)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Uploaded %s\n", result.Filename)
			if result.Public {
				fmt.Fprintf(cmd.ErrOrStderr(), "⚠ Uploaded to a public URL readable by anyone: %s\n", result.AssetURL)
			}
			attachmentTitle := title
			if attachmentTitle == "" {
				attachmentTitle = result.Filename
			}
			input := map[string]any{
				"issueId": issueUUID,
				"title":   attachmentTitle,
				"url":     result.AssetURL,
			}
			if comment != "" {
				input["commentBody"] = comment
			}
			data, err := client.RequestRaw(context.Background(), `
mutation AttachmentCreate($input: AttachmentCreateInput!) {
  attachmentCreate(input: $input) {
    success
    attachment { id url title }
  }
}`, map[string]any{"input": input})
			if err != nil {
				errors.HandleError(err, "Failed to attach file")
				os.Exit(1)
			}
			var parsed struct {
				AttachmentCreate struct {
					Success    bool `json:"success"`
					Attachment *struct {
						Title string `json:"title"`
						URL   string `json:"url"`
					} `json:"attachment"`
				} `json:"attachmentCreate"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil || !parsed.AttachmentCreate.Success || parsed.AttachmentCreate.Attachment == nil {
				errors.HandleError(errors.NewCliError("Failed to create attachment"), "Failed to attach file")
				os.Exit(1)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Sidebar link attachment created: %s\n", parsed.AttachmentCreate.Attachment.Title)
			fmt.Fprintln(cmd.OutOrStdout(), parsed.AttachmentCreate.Attachment.URL)
		},
	}
	cmd.Flags().StringVarP(&title, "title", "t", "", "Custom title for the attachment")
	cmd.Flags().StringVarP(&comment, "comment", "c", "", "Create a linked comment with this body")
	cmd.Flags().BoolVar(&makePublic, "public", false, "Upload to a public URL")
	return cmd
}

func newAgentSessionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent-session",
		Short: "Manage agent sessions for an issue",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newAgentSessionListCommand(), newAgentSessionViewCommand())
	return cmd
}

func newAgentSessionListCommand() *cobra.Command {
	var jsonOut bool
	var status string
	cmd := &cobra.Command{
		Use:   "list [issueId]",
		Short: "List agent sessions for an issue",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			provided := ""
			if len(args) > 0 {
				provided = args[0]
			}
			resolved := mustResolveIssue(provided, "Failed to list agent sessions")
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to list agent sessions")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), `
query GetIssueAgentSessions($issueId: String!) {
  issue(id: $issueId) {
    comments(first: 100) {
      nodes {
        agentSession {
          id status type createdAt startedAt endedAt summary
          creator { name }
          appUser { name }
        }
      }
      pageInfo { hasNextPage endCursor }
    }
  }
}`, map[string]any{"issueId": resolved})
			if err != nil {
				errors.HandleError(err, "Failed to list agent sessions")
				os.Exit(1)
			}
			var parsed struct {
				Issue *struct {
					Comments struct {
						Nodes []struct {
							AgentSession *json.RawMessage `json:"agentSession"`
						} `json:"nodes"`
					} `json:"comments"`
				} `json:"issue"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil || parsed.Issue == nil {
				errors.HandleError(errors.NewNotFoundError("Issue", resolved), "Failed to list agent sessions")
				os.Exit(1)
			}
			sessions := make([]json.RawMessage, 0)
			for _, n := range parsed.Issue.Comments.Nodes {
				if n.AgentSession == nil {
					continue
				}
				if status != "" {
					var s struct {
						Status string `json:"status"`
					}
					_ = json.Unmarshal(*n.AgentSession, &s)
					if s.Status != status {
						continue
					}
				}
				sessions = append(sessions, *n.AgentSession)
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(sessions)
				return
			}
			for _, raw := range sessions {
				var s struct {
					ID      string  `json:"id"`
					Status  string  `json:"status"`
					Type    string  `json:"type"`
					Summary *string `json:"summary"`
				}
				_ = json.Unmarshal(raw, &s)
				sum := ""
				if s.Summary != nil {
					sum = *s.Summary
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\n", s.Status, s.Type, s.ID, sum)
			}
		},
	}
	cmd.Flags().BoolVarP(&jsonOut, "json", "j", false, "Output as JSON")
	cmd.Flags().StringVar(&status, "status", "", "Filter by session status")
	return cmd
}

func newAgentSessionViewCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:     "view <sessionId>",
		Aliases: []string{"v"},
		Short:   "View agent session details",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			client, err := graphql.NewClient()
			if err != nil {
				errors.HandleError(err, "Failed to view agent session")
				os.Exit(1)
			}
			data, err := client.RequestRaw(context.Background(), `
query GetAgentSessionDetails($id: String!) {
  agentSession(id: $id) {
    id status type createdAt updatedAt startedAt endedAt dismissedAt
    summary externalLink
    creator { name }
    appUser { name }
    issue { identifier title url }
  }
}`, map[string]any{"id": args[0]})
			if err != nil {
				errors.HandleError(err, "Failed to view agent session")
				os.Exit(1)
			}
			var parsed struct {
				AgentSession *struct {
					ID      string  `json:"id"`
					Status  string  `json:"status"`
					Type    string  `json:"type"`
					Summary *string `json:"summary"`
					Issue   *struct {
						Identifier string `json:"identifier"`
						Title      string `json:"title"`
						URL        string `json:"url"`
					} `json:"issue"`
				} `json:"agentSession"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil || parsed.AgentSession == nil {
				errors.HandleError(errors.NewNotFoundError("Agent session", args[0]), "Failed to view agent session")
				os.Exit(1)
			}
			if jsonOut {
				var pretty any
				_ = json.Unmarshal(data, &pretty)
				if m, ok := pretty.(map[string]any); ok {
					pretty = m["agentSession"]
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(pretty)
				return
			}
			s := parsed.AgentSession
			fmt.Fprintf(cmd.OutOrStdout(), "# Agent Session %s\n\n", s.ID)
			fmt.Fprintf(cmd.OutOrStdout(), "**Status:** %s\n", s.Status)
			fmt.Fprintf(cmd.OutOrStdout(), "**Type:** %s\n", s.Type)
			if s.Issue != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "**Issue:** %s - %s\n", s.Issue.Identifier, s.Issue.Title)
			}
			if s.Summary != nil && *s.Summary != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "\n%s\n", *s.Summary)
			}
		},
	}
	cmd.Flags().BoolVarP(&jsonOut, "json", "j", false, "Output as JSON")
	return cmd
}

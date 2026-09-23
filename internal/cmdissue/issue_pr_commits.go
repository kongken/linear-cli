package cmdissue

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/kongken/linear-cli/internal/errors"
	"github.com/kongken/linear-cli/internal/linear"
	"github.com/spf13/cobra"
)

func newPullRequestCommand() *cobra.Command {
	var (
		base, head, customTitle string
		draft, web              bool
	)
	cmd := &cobra.Command{
		Use:   "pull-request [issueId]",
		Short: "Create a pull request for the issue",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			provided := ""
			if len(args) > 0 {
				provided = args[0]
			}
			resolved := mustResolveIssue(provided, "Failed to create pull request")
			issue, err := linear.FetchIssueTitleURL(context.Background(), resolved)
			if err != nil {
				errors.HandleError(err, "Failed to create pull request")
				os.Exit(1)
			}
			title := resolved + " " + issue.Title
			if customTitle != "" {
				title = resolved + " " + customTitle
			}
			ghArgs := []string{"pr", "create", "--title", title, "--body", issue.URL}
			if base != "" {
				ghArgs = append(ghArgs, "--base", base)
			}
			if head != "" {
				ghArgs = append(ghArgs, "--head", head)
			}
			if draft {
				ghArgs = append(ghArgs, "--draft")
			}
			if web {
				ghArgs = append(ghArgs, "--web")
			}
			c := exec.Command("gh", ghArgs...)
			c.Stdout = cmd.OutOrStdout()
			c.Stderr = os.Stderr
			c.Stdin = os.Stdin
			if err := c.Run(); err != nil {
				errors.HandleError(
					errors.NewCliError("gh pr create failed: "+err.Error()),
					"Failed to create pull request",
				)
				os.Exit(1)
			}
		},
	}
	cmd.Flags().StringVar(&base, "base", "", "Base branch")
	cmd.Flags().StringVar(&head, "head", "", "Head branch")
	cmd.Flags().StringVar(&customTitle, "title", "", "Custom PR title (issue id still prefixed)")
	cmd.Flags().BoolVar(&draft, "draft", false, "Create as draft")
	cmd.Flags().BoolVar(&web, "web", false, "Open the web browser")
	return cmd
}

func newCommitsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "commits [issueId]",
		Short: "List commits for an issue",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			provided := ""
			if len(args) > 0 {
				provided = args[0]
			}
			resolved := mustResolveIssue(provided, "Failed to list commits")
			// Best-effort: git log messages mentioning the issue id.
			out, err := exec.Command("git", "log", "--oneline", "--all", "--grep="+resolved, "-i").CombinedOutput()
			if err != nil {
				errors.HandleError(
					errors.NewCliError(strings.TrimSpace(string(out))),
					"Failed to list commits",
				)
				os.Exit(1)
			}
			text := strings.TrimSpace(string(out))
			if text == "" {
				fmt.Fprintln(cmd.OutOrStdout(), "No commits found mentioning", resolved)
				return
			}
			fmt.Fprintln(cmd.OutOrStdout(), text)
		},
	}
	return cmd
}

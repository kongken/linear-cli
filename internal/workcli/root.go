package workcli

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/kongken/workctl/internal/cli"
	"github.com/kongken/workctl/internal/cmdplane"
	clierrors "github.com/kongken/workctl/internal/errors"
	"github.com/kongken/workctl/internal/version"
	"github.com/spf13/cobra"
)

// NewRoot creates an independent command tree; the legacy Linear tree is reused intact.
func NewRoot() *cobra.Command {
	root := &cobra.Command{Use: "workctl", Short: "Manage work in Linear and Plane", Version: version.Version, SilenceErrors: true, SilenceUsage: true}
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return clierrors.NewValidationError(err.Error()) })
	root.AddCommand(cli.NewRoot(), cmdplane.New())
	return root
}

// Run renders one error at the process boundary. Plane command actions return errors.
func Run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) int {
	root := NewRoot()
	root.SetArgs(args)
	root.SetIn(in)
	root.SetOut(out)
	root.SetErr(errOut)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return 0
	}
	clierrors.HandleErrorTo(errOut, err, "")
	if errors.Is(err, context.Canceled) {
		return 130
	}
	var validation *clierrors.ValidationError
	if errors.As(err, &validation) || strings.HasPrefix(err.Error(), "unknown command ") {
		return 2
	}
	return 1
}

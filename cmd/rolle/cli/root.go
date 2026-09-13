// Package cli defines the rolle command tree.
package cli

import (
	"context"
	"os"
	"os/signal"

	"github.com/spf13/cobra"

	"github.com/nateships/rolle/internal/app"
	"github.com/nateships/rolle/internal/debug"
	"github.com/nateships/rolle/internal/version"
)

var (
	svc       *app.Service
	debugFlag bool
	// newService builds the service for a command run. Tests replace it.
	newService = app.Default
)

// Root returns the rolle command.
func Root() *cobra.Command {
	root := &cobra.Command{
		Use:           "rolle",
		Short:         "Assume any role, any cloud",
		Version:       version.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if debugFlag {
				debug.Enable()
			}
			s, err := newService()
			if err != nil {
				return err
			}
			svc = s
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			// The first Ctrl-C cancels the context. Release the handler then,
			// so a second Ctrl-C ends a command that does not read the context.
			go func() { <-ctx.Done(); stop() }()
			cmd.SetContext(ctx)
			cobra.OnFinalize(stop)
			return nil
		},
	}
	root.PersistentFlags().BoolVar(&debugFlag, "debug", false, "verbose diagnostics (same as ROLLE_DEBUG=1)")
	root.PersistentFlags().BoolVar(&jsonFlag, "json", false, "machine-readable output for list, status, and start")
	root.SetContext(context.Background())
	root.AddCommand(integrationCmd(), sessionCmd(), tagCmd(), aliasCmd(), startCmd(), stopCmd(), credsCmd(), envCmd(), tokenCmd(), consoleCmd(), statusCmd(), resetCmd(), shellCmd(), supportCmd())
	return root
}

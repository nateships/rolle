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
			cmd.SetContext(ctx)
			cobra.OnFinalize(stop)
			return nil
		},
	}
	root.PersistentFlags().BoolVar(&debugFlag, "debug", false, "verbose diagnostics (same as ROLLE_DEBUG=1)")
	root.SetContext(context.Background())
	root.AddCommand(integrationCmd(), sessionCmd(), startCmd(), stopCmd(), credsCmd(), envCmd(), consoleCmd(), statusCmd(), resetCmd(), shellCmd())
	return root
}

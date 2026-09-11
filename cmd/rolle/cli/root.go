// Package cli defines the rolle command tree.
package cli

import (
	"context"
	"os"
	"os/signal"

	"github.com/spf13/cobra"

	"github.com/nateships/rolle/internal/app"
	"github.com/nateships/rolle/internal/version"
)

var svc *app.Service

// Root returns the rolle command.
func Root() *cobra.Command {
	root := &cobra.Command{
		Use:           "rolle",
		Short:         "Assume any role, any cloud",
		Version:       version.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			s, err := app.Default()
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
	root.SetContext(context.Background())
	root.AddCommand(integrationCmd(), sessionCmd(), startCmd(), stopCmd(), credsCmd(), envCmd(), consoleCmd(), statusCmd())
	return root
}

package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/nateships/rolle/internal/debug"
	"github.com/nateships/rolle/internal/support"
	"github.com/nateships/rolle/internal/version"
)

func supportCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "support",
		Short: "Write a redacted support bundle to attach to a bug report",
		RunE: func(_ *cobra.Command, _ []string) error {
			w, err := svc.Load()
			if err != nil {
				return err
			}
			st, _ := svc.Settings()
			if out == "" {
				out = support.DefaultPath(time.Now())
			}
			in := support.Inputs{
				Version:       version.Version,
				App:           "cli",
				Workspace:     w,
				Settings:      st,
				WorkspacePath: svc.WorkspacePath,
				CacheDir:      svc.Cache.Dir,
				AWSConfigPath: svc.AWSConfigPath,
				Log:           debug.Recent(),
			}
			if err := support.WriteFile(out, in); err != nil {
				return err
			}
			fmt.Println(out)
			fmt.Println("Attach it to a bug report: https://github.com/nateships/rolle/issues/new/choose")
			return nil
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "where to write the zip (default: Downloads)")
	return cmd
}

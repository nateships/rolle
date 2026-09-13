package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// cleanupCmd lists and removes the static keys in the shared credentials
// file. Tools read those before any rolle profile of the same name.
func cleanupCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "cleanup [profile ...]",
		Short: "List or remove profiles with static keys in ~/.aws/credentials",
		Long: "List the profiles of the shared credentials file that hold static keys. " +
			"Name the profiles to remove their keys, or pass --all. Tools read static keys before any rolle profile of the same name.",
		RunE: func(_ *cobra.Command, args []string) error {
			st := svc.StaticProfiles()
			if len(args) == 0 && !all {
				if jsonFlag {
					return writeJSON(st)
				}
				if len(st.Profiles) == 0 {
					fmt.Printf("no static keys in %s\n", st.Path)
					return nil
				}
				fmt.Printf("profiles with static keys in %s:\n", st.Path)
				for _, p := range st.Profiles {
					fmt.Printf("  %s\n", p.Name)
				}
				fmt.Println("rolle cleanup <profile> removes a section's keys; --all removes every one.")
				return nil
			}
			names := args
			if all {
				names = names[:0]
				for _, p := range st.Profiles {
					names = append(names, p.Name)
				}
			}
			for _, n := range names {
				if err := svc.RemoveStaticProfile(n); err != nil {
					return err
				}
				fmt.Printf("removed the static keys of %s from %s\n", n, st.Path)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "remove the static keys of every section")
	return cmd
}

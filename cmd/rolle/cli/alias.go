package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/nateships/rolle/internal/app"
)

// aliasCmd names Identity Center accounts and permission sets. An alias
// replaces the raw name in every session name, and sync keeps it.
func aliasCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "alias", Short: "Name Identity Center accounts and permission sets"}
	var clear bool
	set := func(kind app.AliasKind, use, short string) *cobra.Command {
		c := &cobra.Command{
			Use:   use,
			Short: short,
			Args:  cobra.RangeArgs(1, 2),
			RunE: func(cmd *cobra.Command, args []string) error {
				alias := ""
				if len(args) == 2 {
					alias = args[1]
				}
				if !clear && alias == "" {
					return fmt.Errorf("give an alias, or --clear to remove one")
				}
				if clear {
					alias = ""
				}
				return svc.SetAlias(kind, args[0], alias)
			},
		}
		c.Flags().BoolVar(&clear, "clear", false, "remove the alias and restore the original name")
		return c
	}
	cmd.AddCommand(
		set(app.AliasAccount, "account <account-id|name> <alias>", "Name an account; every role of it takes the name"),
		set(app.AliasRole, "role <permission-set|alias> <alias>", "Name a permission set in every account"),
		&cobra.Command{
			Use:   "list",
			Short: "List aliases",
			Args:  cobra.NoArgs,
			RunE: func(*cobra.Command, []string) error {
				list, err := svc.Aliases()
				if err != nil {
					return err
				}
				if jsonFlag {
					if list == nil {
						list = []app.Alias{}
					}
					return writeJSON(list)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
				fmt.Fprintln(tw, "KIND\tKEY\tNAME\tALIAS")
				for _, a := range list {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", a.Kind, a.Key, a.Name, a.Alias)
				}
				return tw.Flush()
			},
		},
	)
	return cmd
}

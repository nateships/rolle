package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/nateships/rolle/internal/core"
)

func tagCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "tag", Short: "Manage the sidebar tags that group sessions"}
	var add, set core.Tag
	addCmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Create a tag",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			add.Name = args[0]
			return svc.AddTag(add)
		},
	}
	addCmd.Flags().StringVar(&add.Color, "color", "", "a #rrggbb value")
	addCmd.Flags().StringVar(&add.Icon, "icon", "", "a Lucide icon name, for example shield")
	setCmd := &cobra.Command{
		Use:   "set <name>",
		Short: "Change a tag's name, color, or icon",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			w, err := svc.Load()
			if err != nil {
				return err
			}
			i := -1
			for k, t := range w.Tags {
				if strings.EqualFold(t.Name, args[0]) {
					i = k
				}
			}
			if i < 0 {
				return fmt.Errorf("tag %s: %w", args[0], core.ErrNotFound)
			}
			t := w.Tags[i]
			if set.Name != "" {
				t.Name = set.Name
			}
			if set.Color != "" {
				t.Color = set.Color
			}
			if set.Icon != "" {
				t.Icon = set.Icon
			}
			return svc.UpdateTag(args[0], t)
		},
	}
	setCmd.Flags().StringVar(&set.Name, "name", "", "new name")
	setCmd.Flags().StringVar(&set.Color, "color", "", "a #rrggbb value")
	setCmd.Flags().StringVar(&set.Icon, "icon", "", "a Lucide icon name, for example shield")
	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List tags in sidebar order",
			RunE: func(_ *cobra.Command, _ []string) error {
				w, err := svc.Load()
				if err != nil {
					return err
				}
				if jsonFlag {
					return writeJSON(w.Tags)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
				fmt.Fprintln(tw, "NAME\tCOLOR\tICON")
				for _, t := range w.Tags {
					fmt.Fprintf(tw, "%s\t%s\t%s\n", t.Name, t.Color, t.Icon)
				}
				return tw.Flush()
			},
		},
		addCmd,
		setCmd,
		&cobra.Command{
			Use:   "remove <name>",
			Short: "Delete a tag and take it off every session",
			Args:  cobra.ExactArgs(1),
			RunE:  func(_ *cobra.Command, args []string) error { return svc.RemoveTag(args[0]) },
		},
		&cobra.Command{
			Use:   "move <name> <index>",
			Short: "Put a tag at a position in the sidebar, counting from 0",
			Args:  cobra.ExactArgs(2),
			RunE: func(_ *cobra.Command, args []string) error {
				i, err := strconv.Atoi(args[1])
				if err != nil {
					return fmt.Errorf("index %q is not a number", args[1])
				}
				return svc.MoveTag(args[0], i)
			},
		},
	)
	return cmd
}

// sessionTagCmd builds session tag and session untag.
func sessionTagCmd(on bool) *cobra.Command {
	verb, short := "tag", "Add a session to a tag"
	if !on {
		verb, short = "untag", "Take a session off a tag"
	}
	return &cobra.Command{
		Use:   verb + " <session> <tag>",
		Short: short,
		Args:  cobra.ExactArgs(2),
		RunE:  func(_ *cobra.Command, args []string) error { return svc.SetSessionTag(args[0], args[1], on) },
	}
}

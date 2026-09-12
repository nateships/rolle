package cli

import (
	"errors"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/nateships/rolle/internal/app"
	"github.com/nateships/rolle/internal/core"
)

func sessionCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "session", Aliases: []string{"sess"}, Short: "Manage sessions"}
	cmd.AddCommand(sessionListCmd(), sessionAddCmd(), sessionRemoveCmd(), sessionProfileCmd(), sessionRegionCmd(), sessionHideCmd(true), sessionHideCmd(false), sessionTagCmd(true), sessionTagCmd(false))
	return cmd
}

func sessionListCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List sessions",
		RunE: func(_ *cobra.Command, _ []string) error {
			w, err := svc.Refresh()
			if err != nil {
				return err
			}
			sessions := w.Sessions
			if !all {
				sessions = nil
				for _, s := range w.Sessions {
					if !s.Hidden || s.Status == core.StatusActive {
						sessions = append(sessions, s)
					}
				}
			}
			if jsonFlag {
				return writeJSON(sessionsOut(w, sessions))
			}
			return printSessions(sessions)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "include hidden sessions")
	return cmd
}

// sessionHideCmd builds hide and unhide: one session by name or ID, or every
// role of an Identity Center account with --account.
func sessionHideCmd(hide bool) *cobra.Command {
	var account, all bool
	verb, short := "hide", "Keep a session out of the lists and the tray"
	if !hide {
		verb, short = "unhide", "Show a hidden session again"
	}
	cmd := &cobra.Command{
		Use:   verb + " <session|account-id>",
		Short: short + "; --account applies to every role of an AWS account",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if all {
				return svc.UnhideAll()
			}
			if len(args) != 1 {
				return errors.New("name a session, or an account with --account")
			}
			if !account {
				return svc.SetHidden(args[0], hide)
			}
			w, err := svc.Load()
			if err != nil {
				return err
			}
			n := 0
			for _, in := range w.Integrations {
				if err := svc.SetAccountHidden(in.ID, args[0], hide); err == nil {
					n++
				}
			}
			if n == 0 {
				return fmt.Errorf("no Identity Center roles in account %s", args[0])
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&account, "account", false, "treat the argument as an AWS account ID")
	if !hide {
		cmd.Flags().BoolVar(&all, "all", false, "show every hidden session again")
	}
	return cmd
}

func printSessions(sessions []core.Session) error {
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tKIND\tREGION\tSTATUS\tEXPIRES\tID")
	for _, s := range sessions {
		exp := ""
		if s.Expires != nil {
			exp = time.Until(*s.Expires).Truncate(time.Minute).String()
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", s.Name, s.Kind, s.Region, s.Status, exp, s.ID)
	}
	return tw.Flush()
}

func sessionAddCmd() *cobra.Command {
	add := &cobra.Command{Use: "add", Short: "Add a session"}

	var ar app.AddAssumeRoleInput
	assume := &cobra.Command{
		Use:   "assume-role",
		Short: "Assume a role from another session",
		RunE: func(_ *cobra.Command, _ []string) error {
			s, err := svc.AddAssumeRole(ar)
			if err != nil {
				return err
			}
			fmt.Printf("added %s (%s)\n", s.Name, s.ID)
			return nil
		},
	}
	assume.Flags().StringVar(&ar.Name, "name", "", "session name")
	assume.Flags().StringVar(&ar.RoleARN, "role-arn", "", "role to assume")
	assume.Flags().StringVar(&ar.SourceRef, "source", "", "session that provides the source credentials")
	assume.Flags().StringVar(&ar.Region, "region", "", "default region")
	assume.Flags().StringVar(&ar.ExternalID, "external-id", "", "external ID, if the trust policy needs one")
	assume.Flags().StringVar(&ar.Profile, "profile", "", "AWS profile name (empty uses the shared default profile)")
	for _, f := range []string{"name", "role-arn", "source", "region"} {
		_ = assume.MarkFlagRequired(f)
	}

	var iu app.AddIAMUserInput
	iam := &cobra.Command{
		Use:   "iam-user",
		Short: "Add an IAM user with an access key",
		RunE: func(_ *cobra.Command, _ []string) error {
			if iu.Key.SecretAccessKey == "" {
				fmt.Fprint(os.Stderr, "Secret access key: ")
				b, err := term.ReadPassword(int(os.Stdin.Fd()))
				fmt.Fprintln(os.Stderr)
				if err != nil {
					return err
				}
				iu.Key.SecretAccessKey = string(b)
			}
			s, err := svc.AddIAMUser(iu)
			if err != nil {
				return err
			}
			fmt.Printf("added %s (%s)\n", s.Name, s.ID)
			return nil
		},
	}
	iam.Flags().StringVar(&iu.Name, "name", "", "session name")
	iam.Flags().StringVar(&iu.Region, "region", "", "default region")
	iam.Flags().StringVar(&iu.Key.AccessKeyID, "access-key-id", "", "access key ID")
	iam.Flags().StringVar(&iu.Key.SecretAccessKey, "secret-access-key", "", "secret access key (prompted when omitted)")
	iam.Flags().StringVar(&iu.MFADevice, "mfa-device", "", "MFA device ARN or serial")
	iam.Flags().StringVar(&iu.Profile, "profile", "", "AWS profile name (empty uses the shared default profile)")
	for _, f := range []string{"name", "region", "access-key-id"} {
		_ = iam.MarkFlagRequired(f)
	}

	add.AddCommand(assume, iam, sessionAddGCPImpersonateCmd())
	return add
}

func sessionRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <session>",
		Short: "Remove a session",
		Args:  cobra.ExactArgs(1),
		RunE:  func(_ *cobra.Command, args []string) error { return svc.RemoveSession(args[0]) },
	}
}

func sessionProfileCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "profile <session> [name]",
		Short: "Set the AWS profile name for a session (omit the name to restore the default)",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(_ *cobra.Command, args []string) error {
			name := ""
			if len(args) == 2 {
				name = args[1]
			}
			return svc.SetProfile(args[0], name)
		},
	}
}

func sessionRegionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "region <session> <region>",
		Short: "Change the region of an AWS session",
		Args:  cobra.ExactArgs(2),
		RunE:  func(_ *cobra.Command, args []string) error { return svc.SetRegion(args[0], args[1]) },
	}
}

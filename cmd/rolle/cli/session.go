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
	"github.com/nateships/rolle/internal/awsconfig"
	"github.com/nateships/rolle/internal/core"
)

func sessionCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "session", Aliases: []string{"sess"}, Short: "Manage sessions"}
	cmd.AddCommand(sessionListCmd(), sessionAddCmd(), sessionRemoveCmd(), sessionProfileCmd(), sessionFixProfileCmd(), sessionRegionCmd(), sessionHideCmd(true), sessionHideCmd(false), sessionTagCmd(true), sessionTagCmd(false))
	return cmd
}

func sessionListCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List sessions",
		RunE: func(cmd *cobra.Command, _ []string) error {
			w, err := svc.RefreshContext(cmd.Context())
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
				} else if !errors.Is(err, core.ErrNotFound) {
					return err
				}
			}
			if n == 0 {
				return fmt.Errorf("no Identity Center roles in account %s: %w", args[0], core.ErrNotFound)
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
	var fromProfile string
	iam := &cobra.Command{
		Use:   "iam-user",
		Short: "Add an IAM user with an access key",
		Long: "Add an IAM user with an access key. --from-profile reads the key of a profile in ~/.aws/credentials " +
			"and names the session and its profile after it; --name, --region, --mfa-device, and --profile override that.",
		RunE: func(_ *cobra.Command, _ []string) error {
			if fromProfile != "" {
				in, err := svc.IAMUserFromProfile(fromProfile)
				if err != nil {
					return err
				}
				if iu.Name != "" {
					in.Name = iu.Name
				}
				if iu.Region != "" {
					in.Region = iu.Region
				}
				if iu.MFADevice != "" {
					in.MFADevice = iu.MFADevice
				}
				if iu.Profile != "" {
					in.Profile = iu.Profile
				}
				iu = in
			} else if iu.Name == "" || iu.Region == "" {
				return fmt.Errorf("--name and --region are required without --from-profile")
			}
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
	iam.Flags().StringVar(&fromProfile, "from-profile", "", "read the key of this profile in ~/.aws/credentials")
	iam.MarkFlagsOneRequired("access-key-id", "from-profile")
	iam.MarkFlagsMutuallyExclusive("access-key-id", "from-profile")
	iam.MarkFlagsMutuallyExclusive("secret-access-key", "from-profile")

	var lg app.AddAWSLoginInput
	login := &cobra.Command{
		Use:   "aws-login",
		Short: "Add a console login, the flow behind `aws login`",
		Long: "Add a session that signs in with console credentials in the browser: an IAM user, the root user, or IAM federation. " +
			"The first `rolle start` opens the browser; a refresh token then renews the short-lived credentials for up to 12 hours. " +
			"The identity needs the SignInLocalDevelopmentAccess managed policy, unless it is the root user.",
		RunE: func(_ *cobra.Command, _ []string) error {
			s, err := svc.AddAWSLogin(lg)
			if err != nil {
				return err
			}
			fmt.Printf("added %s (%s)\n", s.Name, s.ID)
			return nil
		},
	}
	login.Flags().StringVar(&lg.Name, "name", "", "session name")
	login.Flags().StringVar(&lg.Region, "region", "", "region to sign in to, and the default region")
	login.Flags().StringVar(&lg.Profile, "profile", "", "AWS profile name (empty uses the shared default profile)")
	for _, f := range []string{"name", "region"} {
		_ = login.MarkFlagRequired(f)
	}

	add.AddCommand(assume, iam, login, sessionAddGCPImpersonateCmd())
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

// sessionFixProfileCmd removes the static keys that keep tools from using a
// session's rolle profile.
func sessionFixProfileCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "fix-profile <session>",
		Short: "Remove the static keys in ~/.aws/credentials that shadow the session's profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			w, err := svc.Load()
			if err != nil {
				return err
			}
			sess, err := app.FindSession(w, args[0])
			if err != nil {
				return err
			}
			name := app.ProfileName(sess)
			sh := svc.ProfileShadow(name)
			if sh == nil {
				fmt.Printf("profile %s is not shadowed\n", name)
				return nil
			}
			if !sh.Fixable {
				return fmt.Errorf("another tool configures profile %q in %s; use another profile name", name, awsconfig.Display(sh.Path))
			}
			if err := svc.RemoveStaticProfile(name); err != nil {
				return err
			}
			fmt.Printf("removed the static keys of profile %s from %s\n", name, sh.Path)
			return nil
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

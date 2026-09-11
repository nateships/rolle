package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/nateships/rolle/internal/browser"
)

func integrationCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "integration", Aliases: []string{"int"}, Short: "Manage identity sources"}
	cmd.AddCommand(integrationAddCmd(), integrationListCmd(), integrationLoginCmd(), integrationLogoutCmd(), integrationSyncCmd(), integrationRemoveCmd())
	return cmd
}

func integrationAddCmd() *cobra.Command {
	add := &cobra.Command{Use: "add", Short: "Add an identity source"}
	var alias, startURL, region string
	sso := &cobra.Command{
		Use:   "aws-sso",
		Short: "Add an AWS IAM Identity Center portal",
		RunE: func(cmd *cobra.Command, _ []string) error {
			in, err := svc.AddAWSSSO(alias, startURL, region)
			if err != nil {
				return err
			}
			fmt.Printf("added %s (%s)\nnext: rolle integration login %s\n", in.Alias, in.ID, in.Alias)
			return nil
		},
	}
	sso.Flags().StringVar(&alias, "alias", "", "short name for this portal")
	sso.Flags().StringVar(&startURL, "start-url", "", "portal start URL, for example https://acme.awsapps.com/start")
	sso.Flags().StringVar(&region, "region", "", "region the portal is hosted in")
	_ = sso.MarkFlagRequired("alias")
	_ = sso.MarkFlagRequired("start-url")
	_ = sso.MarkFlagRequired("region")
	add.AddCommand(sso)
	return add
}

func integrationListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List identity sources",
		RunE: func(cmd *cobra.Command, _ []string) error {
			w, err := svc.Load()
			if err != nil {
				return err
			}
			tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "ALIAS\tCLOUD\tSTATE\tID")
			for _, in := range w.Integrations {
				state := "logged out"
				if in.AWSSSO != nil && in.AWSSSO.TokenExpires != nil {
					state = "logged in until " + in.AWSSSO.TokenExpires.Local().Format("Jan 2 15:04")
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", in.Alias, in.Cloud, state, in.ID)
			}
			return tw.Flush()
		},
	}
}

func integrationLoginCmd() *cobra.Command {
	var noBrowser bool
	cmd := &cobra.Command{
		Use:   "login <integration>",
		Short: "Sign in and discover roles",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			auth, err := svc.SSOLogin(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			fmt.Printf("Open %s\nand confirm code %s\n", auth.VerificationURI, auth.UserCode)
			if !noBrowser {
				_ = browser.Open(auth.VerificationURI)
			}
			fmt.Println("Waiting for approval...")
			if err := auth.Wait(cmd.Context()); err != nil {
				return err
			}
			added, err := svc.FinishSSOLogin(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			fmt.Printf("Logged in. %d new role(s) discovered.\n", len(added))
			for _, s := range added {
				fmt.Printf("  %s\n", s.Name)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "print the URL instead of opening a browser")
	return cmd
}

func integrationLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout <integration>",
		Short: "Sign out and stop its sessions",
		Args:  cobra.ExactArgs(1),
		RunE:  func(_ *cobra.Command, args []string) error { return svc.SSOLogout(args[0]) },
	}
}

func integrationSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync <integration>",
		Short: "Rediscover accounts and roles",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			added, err := svc.SyncSSO(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			fmt.Printf("%d new role(s)\n", len(added))
			return nil
		},
	}
}

func integrationRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <integration>",
		Short: "Remove an identity source and its sessions",
		Args:  cobra.ExactArgs(1),
		RunE:  func(_ *cobra.Command, args []string) error { return svc.RemoveIntegration(args[0]) },
	}
}

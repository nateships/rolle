package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/nateships/rolle/internal/app"
	"github.com/nateships/rolle/internal/aws"
	"github.com/nateships/rolle/internal/browser"
	"github.com/nateships/rolle/internal/core"
)

func azureLogin(cmd *cobra.Command, ref string, noBrowser bool) error {
	var added []core.Session
	var err error
	if noBrowser {
		dc, err := svc.AzureDeviceLogin(cmd.Context(), ref)
		if err != nil {
			return err
		}
		fmt.Println(dc.Message)
		account, err := dc.Wait(cmd.Context())
		if err != nil {
			return err
		}
		added, err = svc.FinishAzureLogin(cmd.Context(), ref, account)
		if err != nil {
			return err
		}
	} else {
		fmt.Println("Complete the sign-in in your browser...")
		added, err = svc.AzureLogin(cmd.Context(), ref)
		if err != nil {
			return err
		}
	}
	fmt.Printf("Logged in. %d new subscription(s) discovered.\n", len(added))
	for _, s := range added {
		fmt.Printf("  %s\n", s.Name)
	}
	return nil
}

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
	add.AddCommand(sso, integrationAddAzureCmd(), integrationAddGCPCmd())
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
				switch {
				case in.AWSSSO != nil && in.AWSSSO.TokenExpires != nil:
					state = "logged in until " + in.AWSSSO.TokenExpires.Local().Format("Jan 2 15:04")
				case in.Azure != nil && in.Azure.Account != "":
					state = in.Azure.Account
				case in.GCP != nil && in.GCP.Account != "":
					state = in.GCP.Account
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
			return loginIntegration(cmd, args[0], noBrowser)
		},
	}
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "print the URL instead of opening a browser")
	return cmd
}

// loginIntegration signs in to one integration and discovers what it grants.
// The sync command falls back to it when the integration has no valid token.
func loginIntegration(cmd *cobra.Command, ref string, noBrowser bool) error {
	w, err := svc.Load()
	if err != nil {
		return err
	}
	in, err := app.FindIntegration(w, ref)
	if err != nil {
		return err
	}
	switch in.Cloud {
	case core.CloudAzure:
		return azureLogin(cmd, ref, noBrowser)
	case core.CloudGCP:
		added, err := svc.SyncGCP(cmd.Context(), ref)
		if err != nil {
			return err
		}
		fmt.Printf("%d new project(s)\n", len(added))
		return nil
	}
	var auth *aws.DeviceAuthorization
	if noBrowser {
		auth, err = svc.SSODeviceLogin(cmd.Context(), ref)
		if err != nil {
			return err
		}
		fmt.Printf("Open %s\nand confirm code %s\n", auth.VerificationURI, auth.UserCode)
	} else {
		auth, err = svc.SSOLogin(cmd.Context(), ref)
		if err != nil {
			return err
		}
		fmt.Printf("Approve the sign-in in your browser. If it did not open:\n%s\n", auth.VerificationURI)
		_ = browser.Open(auth.VerificationURI)
	}
	fmt.Println("Waiting for approval...")
	if err := auth.Wait(cmd.Context()); err != nil {
		return err
	}
	added, err := svc.FinishSSOLogin(cmd.Context(), ref)
	if err != nil {
		return err
	}
	fmt.Printf("Logged in. %d new role(s) discovered.\n", len(added))
	for _, s := range added {
		fmt.Printf("  %s\n", s.Name)
	}
	return nil
}

func integrationLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout <integration>",
		Short: "Sign out and stop its sessions",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			w, err := svc.Load()
			if err != nil {
				return err
			}
			in, err := app.FindIntegration(w, args[0])
			if err != nil {
				return err
			}
			switch in.Cloud {
			case core.CloudAzure:
				return svc.AzureLogout(cmd.Context(), args[0])
			case core.CloudGCP:
				return fmt.Errorf("%s uses the gcloud login; run `gcloud auth application-default revoke` or remove the integration", in.Alias)
			}
			return svc.SSOLogout(args[0])
		},
	}
}

func integrationSyncCmd() *cobra.Command {
	var noBrowser bool
	cmd := &cobra.Command{
		Use:   "sync <integration>",
		Short: "Rediscover accounts and roles, signing in first if needed",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			w, err := svc.Load()
			if err != nil {
				return err
			}
			in, err := app.FindIntegration(w, args[0])
			if err != nil {
				return err
			}
			var added []core.Session
			switch in.Cloud {
			case core.CloudAzure:
				added, err = svc.SyncAzure(cmd.Context(), args[0])
			case core.CloudGCP:
				added, err = svc.SyncGCP(cmd.Context(), args[0])
			default:
				added, err = svc.SyncSSO(cmd.Context(), args[0])
			}
			// GCP signs in through gcloud, so its error already names the command.
			if app.LoginRequired(err) && in.Cloud != core.CloudGCP {
				// Signing in discovers roles too, so the login output replaces the count.
				fmt.Printf("%s needs a sign-in.\n", in.Alias)
				return loginIntegration(cmd, args[0], noBrowser)
			}
			if err != nil {
				return err
			}
			fmt.Printf("%d new session(s)\n", len(added))
			return nil
		},
	}
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "print the URL instead of opening a browser")
	return cmd
}

func integrationRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <integration>",
		Short: "Remove an identity source and its sessions",
		Args:  cobra.ExactArgs(1),
		RunE:  func(_ *cobra.Command, args []string) error { return svc.RemoveIntegration(args[0]) },
	}
}

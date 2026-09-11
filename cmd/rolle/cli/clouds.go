package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nateships/rolle/internal/app"
	"github.com/nateships/rolle/internal/gcp"
)

func integrationAddAzureCmd() *cobra.Command {
	var alias, tenant string
	cmd := &cobra.Command{
		Use:   "azure",
		Short: "Add a Microsoft Entra ID tenant",
		RunE: func(_ *cobra.Command, _ []string) error {
			in, err := svc.AddAzure(alias, tenant)
			if err != nil {
				return err
			}
			fmt.Printf("added %s (%s)\nnext: rolle integration login %s\n", in.Alias, in.ID, in.Alias)
			return nil
		},
	}
	cmd.Flags().StringVar(&alias, "alias", "", "short name for this tenant")
	cmd.Flags().StringVar(&tenant, "tenant", "", "tenant ID or domain (defaults to your home tenant)")
	_ = cmd.MarkFlagRequired("alias")
	return cmd
}

func integrationAddGCPCmd() *cobra.Command {
	var alias string
	cmd := &cobra.Command{
		Use:   "gcp",
		Short: "Add your gcloud Application Default Credentials",
		Long:  "Add your gcloud Application Default Credentials. Run `" + gcp.LoginCommand + "` first if you have not.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			in, added, err := svc.AddGCP(cmd.Context(), alias)
			if err != nil {
				return err
			}
			fmt.Printf("added %s (%s) as %s\n%d project(s) discovered\n", in.Alias, in.ID, in.GCP.Account, len(added))
			return nil
		},
	}
	cmd.Flags().StringVar(&alias, "alias", "gcp", "short name for this account")
	return cmd
}

func sessionAddGCPImpersonateCmd() *cobra.Command {
	var in app.AddGCPImpersonationInput
	cmd := &cobra.Command{
		Use:   "gcp-impersonate",
		Short: "Impersonate a Google Cloud service account",
		RunE: func(_ *cobra.Command, _ []string) error {
			s, err := svc.AddGCPImpersonation(in)
			if err != nil {
				return err
			}
			fmt.Printf("added %s (%s)\n", s.Name, s.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&in.Name, "name", "", "session name")
	cmd.Flags().StringVar(&in.IntegrationRef, "integration", "gcp", "Google Cloud integration alias or ID")
	cmd.Flags().StringVar(&in.ProjectID, "project", "", "project ID")
	cmd.Flags().StringVar(&in.ServiceAccount, "service-account", "", "service account email to impersonate")
	for _, f := range []string{"name", "project", "service-account"} {
		_ = cmd.MarkFlagRequired(f)
	}
	return cmd
}

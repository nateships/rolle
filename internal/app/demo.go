package app

import (
	"os"
	"path/filepath"
	"time"

	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/credcache"
	"github.com/nateships/rolle/internal/secrets"
	"github.com/nateships/rolle/internal/workspace"
)

// Demo builds a Service on a throwaway directory with fictional accounts,
// an in-memory secret store, and fake cached credentials. It touches no real
// workspace, keychain, or AWS config. Use it for screenshots and UI work.
func Demo() (*Service, error) {
	dir, err := os.MkdirTemp("", "rolle-demo-")
	if err != nil {
		return nil, err
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	s := &Service{
		WorkspacePath: filepath.Join(dir, "workspace.json"),
		AWSConfigPath: filepath.Join(dir, "aws-config"),
		Executable:    exe,
		Secrets:       &secrets.Memory{},
		// A short skew keeps the session that expires soon out of renewal,
		// which has no portal to talk to, until it really ends.
		Cache: &credcache.Cache{Store: &secrets.Memory{}, Dir: filepath.Join(dir, "credentials"), Skew: time.Second},
	}
	w, active := demoWorkspace()
	if err := workspace.Save(s.WorkspacePath, w); err != nil {
		return nil, err
	}
	for _, sess := range active {
		creds := core.Credentials{Expiration: sess.Expires}
		if sess.Kind.Cloud() == core.CloudAWS {
			creds.AccessKeyID, creds.SecretAccessKey, creds.SessionToken = "ASIADEMO0000000EXAMPLE", "demo-secret", "demo-session-token"
		} else {
			creds.Token = "demo-token"
		}
		if err := s.Cache.Put(sess.ID, creds); err != nil {
			return nil, err
		}
		if err := s.writeCloudFiles(&sess); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// demoWorkspace returns fictional integrations and sessions, plus the sessions
// that start active.
func demoWorkspace() (*core.Workspace, []core.Session) {
	in := func(d time.Duration) *time.Time { t := time.Now().Add(d); return &t }
	w := &core.Workspace{
		Version:   core.WorkspaceVersion,
		Onboarded: true,
		Integrations: []core.Integration{
			{ID: "acme", Alias: "acme", Cloud: core.CloudAWS, AWSSSO: &core.AWSSSOIntegration{StartURL: "https://acme.awsapps.com/start", Region: "us-east-1", TokenExpires: in(6 * time.Hour), Renews: true}},
			{ID: "acme-eu", Alias: "acme-eu", Cloud: core.CloudAWS, AWSSSO: &core.AWSSSOIntegration{StartURL: "https://acme-eu.awsapps.com/start", Region: "eu-west-1"}},
			{ID: "contoso", Alias: "contoso", Cloud: core.CloudAzure, Azure: &core.AzureIntegration{TenantID: "7a1c2e40-3d5b-4f6a-9b8c-0d1e2f3a4b5c", Account: "nate@contoso.com"}},
			{ID: "gcp", Alias: "gcp", Cloud: core.CloudGCP, GCP: &core.GCPIntegration{Account: "nate@example.com"}},
		},
		Sessions: []core.Session{
			{ID: "s-admin", Name: "Acme Prod/AdministratorAccess", Kind: core.KindAWSSSORole, Region: "us-east-1", IntegrationID: "acme", Status: core.StatusActive, Favorite: true, Expires: in(47 * time.Minute), AWS: &core.AWSSession{AccountID: "123456789012", RoleName: "AdministratorAccess"}},
			// Expires shortly after launch, just outside the two-minute warning
			// lead. This shows the warning, the tray flag, and the Dock badge
			// arrive, and then the deactivation.
			{ID: "s-staging", Name: "Acme Staging/AdministratorAccess", Kind: core.KindAWSSSORole, Region: "us-east-1", IntegrationID: "acme", Status: core.StatusActive, Expires: in(2*time.Minute + 15*time.Second), AWS: &core.AWSSession{AccountID: "456789012345", RoleName: "AdministratorAccess"}},
			// Ends 15 seconds after launch: the expired marker, at once.
			{ID: "s-qa", Name: "Acme QA/ReadOnlyAccess", Kind: core.KindAWSSSORole, Region: "us-east-1", IntegrationID: "acme", Status: core.StatusActive, Expires: in(15 * time.Second), AWS: &core.AWSSession{AccountID: "567890123456", RoleName: "ReadOnlyAccess"}},
			{ID: "s-ro", Name: "Acme Prod/ReadOnlyAccess", Kind: core.KindAWSSSORole, Region: "us-east-1", IntegrationID: "acme", Status: core.StatusInactive, AWS: &core.AWSSession{AccountID: "123456789012", RoleName: "ReadOnlyAccess"}},
			{ID: "s-dev", Name: "Acme Dev/PowerUserAccess", Kind: core.KindAWSSSORole, Region: "us-east-1", IntegrationID: "acme", Status: core.StatusInactive, AWS: &core.AWSSession{AccountID: "210987654321", RoleName: "PowerUserAccess"}},
			{ID: "s-sandbox", Name: "Acme Sandbox/ReadOnlyAccess", Kind: core.KindAWSSSORole, Region: "us-east-1", IntegrationID: "acme", Status: core.StatusInactive, AWS: &core.AWSSession{AccountID: "345678901234", RoleName: "ReadOnlyAccess"}},
			{ID: "s-chain", Name: "prod-admin", Kind: core.KindAWSAssumeRole, Region: "eu-west-1", Status: core.StatusInactive, AWS: &core.AWSSession{Profile: "prod-admin", RoleARN: "arn:aws:iam::123456789012:role/Admin", SourceSessionID: "s-admin"}},
			{ID: "s-iam", Name: "personal", Kind: core.KindAWSIAMUser, Region: "us-west-2", Status: core.StatusInactive, AWS: &core.AWSSession{Profile: "personal", MFADevice: "arn:aws:iam::123456789012:mfa/nate"}},
			{ID: "s-az", Name: "Contoso Production", Kind: core.KindAzure, IntegrationID: "contoso", Status: core.StatusActive, Expires: in(53 * time.Minute), Azure: &core.AzureSession{SubscriptionID: "0f1e2d3c-4b5a-6978-8a9b-0c1d2e3f4a5b", TenantID: "7a1c2e40-3d5b-4f6a-9b8c-0d1e2f3a4b5c"}},
			{ID: "s-gcp", Name: "data-platform", Kind: core.KindGCP, IntegrationID: "gcp", Status: core.StatusInactive, GCP: &core.GCPSession{ProjectID: "data-platform-4821"}},
			{ID: "s-gcp-sa", Name: "deployer", Kind: core.KindGCP, IntegrationID: "gcp", Status: core.StatusInactive, Favorite: true, GCP: &core.GCPSession{ProjectID: "data-platform-4821", ServiceAccount: "deployer@data-platform-4821.iam.gserviceaccount.com"}},
		},
	}
	var active []core.Session
	for _, s := range w.Sessions {
		if s.Status == core.StatusActive {
			active = append(active, s)
		}
	}
	return w, active
}

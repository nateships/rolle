package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nateships/rolle/internal/debug"

	"github.com/nateships/rolle/internal/azure"
	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/discover"
	"github.com/nateships/rolle/internal/gcp"
)

// Discover reports identities other tools already configured on this machine.
func (s *Service) Discover(ctx context.Context) discover.Result { return discover.Scan(ctx) }

// ImportResult describes an imported Identity Center portal.
type ImportResult struct {
	Integration core.Integration `json:"integration"`
	// LoggedIn is true when a valid AWS CLI token was reused and roles were discovered.
	LoggedIn bool           `json:"loggedIn"`
	Sessions []core.Session `json:"sessions"`
}

// ImportAWSSSO registers a portal found in the AWS CLI config. When the CLI
// holds a valid token for it, the token is reused and roles are discovered
// immediately; otherwise the caller runs the normal device login.
func (s *Service) ImportAWSSSO(ctx context.Context, alias, startURL, region string) (ImportResult, error) {
	in, err := s.AddAWSSSO(alias, startURL, region)
	if err != nil {
		return ImportResult{}, err
	}
	res := ImportResult{Integration: in}
	tok, ok := discover.AWSCLITokenFor(startURL)
	if !ok {
		return res, nil
	}
	if err := s.sso(in).StoreImportedToken(tok.AccessToken, tok.RefreshToken, tok.ClientID, tok.ClientSecret, tok.Region, tok.ExpiresAt); err != nil {
		return res, err
	}
	sessions, err := s.FinishSSOLogin(ctx, in.ID)
	if err != nil {
		// The CLI token was not usable; drop it and fall back to a login.
		_ = s.sso(in).Logout()
		debug.Logf("discover", "cli token for %s rejected: %v", startURL, err)
		return res, nil
	}
	res.LoggedIn = true
	res.Sessions = sessions
	return res, nil
}

// AddAzure registers an Entra ID tenant. tenantID may be empty to sign in to
// the user's home tenant.
func (s *Service) AddAzure(alias, tenantID string) (core.Integration, error) {
	w, err := s.Load()
	if err != nil {
		return core.Integration{}, err
	}
	in := core.Integration{ID: newID(), Alias: alias, Cloud: core.CloudAzure, Azure: &core.AzureIntegration{TenantID: tenantID}}
	w.Integrations = append(w.Integrations, in)
	return in, s.Save(w)
}

func (s *Service) azureAuth(in core.Integration) *azure.Auth {
	return &azure.Auth{Integration: in, Secrets: s.Secrets}
}

// AzureLogin signs in through the browser, records the account, and discovers subscriptions.
func (s *Service) AzureLogin(ctx context.Context, ref string) ([]core.Session, error) {
	w, err := s.Load()
	if err != nil {
		return nil, err
	}
	in, err := FindIntegration(w, ref)
	if err != nil {
		return nil, err
	}
	if in.Azure == nil {
		return nil, fmt.Errorf("integration %q is not an Azure tenant", in.Alias)
	}
	account, err := s.azureAuth(*in).Login(ctx)
	if err != nil {
		return nil, err
	}
	in.Azure.Account = account
	if err := s.Save(w); err != nil {
		return nil, err
	}
	return s.SyncAzure(ctx, ref)
}

// AzureDeviceLogin starts a device-code login for terminals without a browser.
func (s *Service) AzureDeviceLogin(ctx context.Context, ref string) (*azure.DeviceCode, error) {
	w, err := s.Load()
	if err != nil {
		return nil, err
	}
	in, err := FindIntegration(w, ref)
	if err != nil {
		return nil, err
	}
	if in.Azure == nil {
		return nil, fmt.Errorf("integration %q is not an Azure tenant", in.Alias)
	}
	return s.azureAuth(*in).StartDeviceLogin(ctx)
}

// FinishAzureLogin records the account after a device-code login and syncs.
func (s *Service) FinishAzureLogin(ctx context.Context, ref, account string) ([]core.Session, error) {
	w, err := s.Load()
	if err != nil {
		return nil, err
	}
	in, err := FindIntegration(w, ref)
	if err != nil {
		return nil, err
	}
	in.Azure.Account = account
	if err := s.Save(w); err != nil {
		return nil, err
	}
	return s.SyncAzure(ctx, ref)
}

// AzureLogout forgets the account and stops its sessions.
func (s *Service) AzureLogout(ctx context.Context, ref string) error {
	w, err := s.Load()
	if err != nil {
		return err
	}
	in, err := FindIntegration(w, ref)
	if err != nil {
		return err
	}
	if err := s.azureAuth(*in).Logout(ctx); err != nil {
		return err
	}
	in.Azure.Account = ""
	for i := range w.Sessions {
		if w.Sessions[i].IntegrationID == in.ID {
			_ = s.deactivate(&w.Sessions[i])
		}
	}
	return s.Save(w)
}

// SyncAzure discovers subscriptions and adds a session per new one.
func (s *Service) SyncAzure(ctx context.Context, ref string) ([]core.Session, error) {
	w, err := s.Load()
	if err != nil {
		return nil, err
	}
	in, err := FindIntegration(w, ref)
	if err != nil {
		return nil, err
	}
	creds, err := s.azureAuth(*in).Token(ctx)
	if err != nil {
		return nil, err
	}
	subs, err := azure.ListSubscriptions(ctx, nil, creds.Token)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var added []core.Session
	for _, sub := range subs {
		seen[sub.ID] = true
		if hasAzureSub(w, in.ID, sub.ID) {
			continue
		}
		sess := core.Session{
			ID: newID(), Name: sub.Name, Kind: core.KindAzure, IntegrationID: in.ID, Status: core.StatusInactive,
			Azure: &core.AzureSession{SubscriptionID: sub.ID, TenantID: sub.TenantID},
		}
		w.Sessions = append(w.Sessions, sess)
		added = append(added, sess)
	}
	kept := w.Sessions[:0]
	for _, sess := range w.Sessions {
		if sess.IntegrationID == in.ID && sess.Kind == core.KindAzure && !seen[sess.Azure.SubscriptionID] {
			_ = s.deactivate(&sess)
			continue
		}
		kept = append(kept, sess)
	}
	w.Sessions = kept
	return added, s.Save(w)
}

func hasAzureSub(w *core.Workspace, integrationID, subID string) bool {
	for _, sess := range w.Sessions {
		if sess.IntegrationID == integrationID && sess.Azure != nil && sess.Azure.SubscriptionID == subID {
			return true
		}
	}
	return false
}

// AddGCP registers the local gcloud Application Default Credentials as an
// identity source and discovers projects.
func (s *Service) AddGCP(ctx context.Context, alias string) (core.Integration, []core.Session, error) {
	acct, err := gcp.DetectAccount(ctx)
	if err != nil {
		return core.Integration{}, nil, err
	}
	w, err := s.Load()
	if err != nil {
		return core.Integration{}, nil, err
	}
	in := core.Integration{ID: newID(), Alias: alias, Cloud: core.CloudGCP, GCP: &core.GCPIntegration{Account: acct.Email}}
	w.Integrations = append(w.Integrations, in)
	if err := s.Save(w); err != nil {
		return core.Integration{}, nil, err
	}
	added, err := s.SyncGCP(ctx, in.ID)
	return in, added, err
}

// SyncGCP discovers active projects and adds a user-identity session per new one.
func (s *Service) SyncGCP(ctx context.Context, ref string) ([]core.Session, error) {
	w, err := s.Load()
	if err != nil {
		return nil, err
	}
	in, err := FindIntegration(w, ref)
	if err != nil {
		return nil, err
	}
	if in.GCP == nil {
		return nil, fmt.Errorf("integration %q is not a Google Cloud account", in.Alias)
	}
	tok, err := gcp.SourceToken(ctx)
	if err != nil {
		return nil, err
	}
	if acct, err := gcp.DetectAccount(ctx); err == nil && acct.Email != "" {
		in.GCP.Account = acct.Email
	}
	projects, err := gcp.ListProjects(ctx, nil, tok.AccessToken)
	if err != nil {
		return nil, err
	}
	var added []core.Session
	for _, p := range projects {
		if hasGCPProject(w, in.ID, p.ID, "") {
			continue
		}
		sess := core.Session{
			ID: newID(), Name: p.Name, Kind: core.KindGCP, IntegrationID: in.ID, Status: core.StatusInactive,
			GCP: &core.GCPSession{ProjectID: p.ID},
		}
		w.Sessions = append(w.Sessions, sess)
		added = append(added, sess)
	}
	return added, s.Save(w)
}

func hasGCPProject(w *core.Workspace, integrationID, projectID, serviceAccount string) bool {
	for _, sess := range w.Sessions {
		if sess.IntegrationID == integrationID && sess.GCP != nil && sess.GCP.ProjectID == projectID && sess.GCP.ServiceAccount == serviceAccount {
			return true
		}
	}
	return false
}

// AddGCPImpersonationInput describes a service account impersonation session.
type AddGCPImpersonationInput struct {
	Name           string `json:"name"`
	IntegrationRef string `json:"integrationRef"`
	ProjectID      string `json:"projectId"`
	ServiceAccount string `json:"serviceAccount"`
}

// AddGCPImpersonation creates a session that impersonates a service account.
func (s *Service) AddGCPImpersonation(in AddGCPImpersonationInput) (core.Session, error) {
	w, err := s.Load()
	if err != nil {
		return core.Session{}, err
	}
	integ, err := FindIntegration(w, in.IntegrationRef)
	if err != nil {
		return core.Session{}, err
	}
	if integ.GCP == nil {
		return core.Session{}, fmt.Errorf("integration %q is not a Google Cloud account", integ.Alias)
	}
	sess := core.Session{
		ID: newID(), Name: in.Name, Kind: core.KindGCP, IntegrationID: integ.ID, Status: core.StatusInactive,
		GCP: &core.GCPSession{ProjectID: in.ProjectID, ServiceAccount: in.ServiceAccount},
	}
	w.Sessions = append(w.Sessions, sess)
	return sess, s.Save(w)
}

// fetchCloud handles Azure and GCP sessions for fetch.
func (s *Service) fetchCloud(ctx context.Context, w *core.Workspace, sess *core.Session) (core.Credentials, error) {
	switch sess.Kind {
	case core.KindAzure:
		in, err := w.Integration(sess.IntegrationID)
		if err != nil {
			return core.Credentials{}, fmt.Errorf("integration for %s: %w", sess.Name, err)
		}
		return s.azureAuth(*in).Token(ctx)
	case core.KindGCP:
		tok, err := gcp.SourceToken(ctx)
		if err != nil {
			return core.Credentials{}, err
		}
		if sess.GCP.ServiceAccount == "" {
			exp := tok.Expiry.UTC()
			if exp.IsZero() {
				exp = time.Now().Add(time.Hour).UTC()
			}
			return core.Credentials{Token: tok.AccessToken, Expiration: &exp}, nil
		}
		return gcp.Impersonate(ctx, nil, tok.AccessToken, sess.GCP.ServiceAccount, time.Hour)
	}
	return core.Credentials{}, fmt.Errorf("session kind %q is not supported yet", sess.Kind)
}

// ConsoleURLFor returns a browser link for any session kind. AWS links are
// federated sign-ins; Azure and GCP links open the portal for the session.
func (s *Service) ConsoleURLFor(ctx context.Context, ref string) (string, error) {
	w, err := s.Load()
	if err != nil {
		return "", err
	}
	sess, err := FindSession(w, ref)
	if err != nil {
		return "", err
	}
	switch sess.Kind.Cloud() {
	case core.CloudAWS:
		return s.ConsoleURL(ctx, ref)
	case core.CloudAzure:
		return azure.PortalURL(sess.Azure.TenantID), nil
	case core.CloudGCP:
		return gcp.ConsoleURL(sess.GCP.ProjectID), nil
	}
	return "", fmt.Errorf("no console for %s", sess.Name)
}

// adcPath is where a GCP impersonation session keeps its ADC file.
func (s *Service) adcPath(sess *core.Session) string {
	return filepath.Join(s.Cache.Dir, "gcp", sess.ID+".json")
}

// writeCloudFiles creates per-session files other tools read, for example the
// impersonated ADC file for GCP service account sessions.
func (s *Service) writeCloudFiles(sess *core.Session) error {
	if sess.Kind == core.KindGCP && sess.GCP != nil && sess.GCP.ServiceAccount != "" {
		return gcp.WriteImpersonatedADC(s.adcPath(sess), sess.GCP.ServiceAccount)
	}
	return nil
}

// removeCloudFiles deletes what writeCloudFiles created.
func (s *Service) removeCloudFiles(sess *core.Session) error {
	if sess.Kind == core.KindGCP {
		if err := os.Remove(s.adcPath(sess)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// EnvVars returns the environment variables a session exports, including
// file paths that only this Service knows.
func (s *Service) EnvVars(sess *core.Session, creds core.Credentials) [][2]string {
	vars := EnvVars(sess, creds)
	if sess.Kind == core.KindGCP && sess.GCP != nil && sess.GCP.ServiceAccount != "" {
		vars = append(vars, [2]string{"GOOGLE_APPLICATION_CREDENTIALS", s.adcPath(sess)})
	}
	return vars
}

// EnvVars returns the environment variables a session exports.
func EnvVars(sess *core.Session, creds core.Credentials) [][2]string {
	switch sess.Kind.Cloud() {
	case core.CloudAWS:
		return [][2]string{
			{"AWS_ACCESS_KEY_ID", creds.AccessKeyID},
			{"AWS_SECRET_ACCESS_KEY", creds.SecretAccessKey},
			{"AWS_SESSION_TOKEN", creds.SessionToken},
			{"AWS_REGION", sess.Region},
			{"AWS_DEFAULT_REGION", sess.Region},
		}
	case core.CloudAzure:
		return [][2]string{
			{"AZURE_SUBSCRIPTION_ID", sess.Azure.SubscriptionID},
			{"AZURE_TENANT_ID", sess.Azure.TenantID},
			{"ARM_SUBSCRIPTION_ID", sess.Azure.SubscriptionID},
			{"ARM_TENANT_ID", sess.Azure.TenantID},
			{"AZURE_ACCESS_TOKEN", creds.Token},
		}
	case core.CloudGCP:
		vars := [][2]string{
			{"CLOUDSDK_CORE_PROJECT", sess.GCP.ProjectID},
			{"GOOGLE_CLOUD_PROJECT", sess.GCP.ProjectID},
			{"CLOUDSDK_AUTH_ACCESS_TOKEN", creds.Token},
			{"GOOGLE_OAUTH_ACCESS_TOKEN", creds.Token},
		}
		if sess.GCP.ServiceAccount != "" {
			vars = append(vars, [2]string{"CLOUDSDK_AUTH_IMPERSONATE_SERVICE_ACCOUNT", sess.GCP.ServiceAccount})
		}
		return vars
	}
	return nil
}

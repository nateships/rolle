package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/nateships/rolle/internal/aws"
	"github.com/nateships/rolle/internal/awsconfig"
	"github.com/nateships/rolle/internal/azure"
	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/debug"
	"github.com/nateships/rolle/internal/discover"
	"github.com/nateships/rolle/internal/gcp"
	"github.com/nateships/rolle/internal/terminal"
)

// Discover reports identities other tools already configured on this machine.
// An access key that a rolle session already holds is marked imported.
func (s *Service) Discover(ctx context.Context) discover.Result {
	r := discover.Scan(ctx)
	if len(r.IAMUsers) > 0 {
		have := s.storedAccessKeyIDs()
		for i := range r.IAMUsers {
			r.IAMUsers[i].Imported = have[r.IAMUsers[i].AccessKeyID]
		}
	}
	return r
}

// storedAccessKeyIDs collects the access key IDs of every IAM user session.
// A key the secret store cannot read is absent.
func (s *Service) storedAccessKeyIDs() map[string]bool {
	out := map[string]bool{}
	w, err := s.Load()
	if err != nil {
		return out
	}
	for _, sess := range w.Sessions {
		if sess.Kind != core.KindAWSIAMUser {
			continue
		}
		if key, err := aws.LoadAccessKey(s.Secrets, sess.ID); err == nil && key.AccessKeyID != "" {
			out[key.AccessKeyID] = true
		}
	}
	return out
}

// IAMUserFromProfile builds the input for an IAM user session from the access
// key of a profile in the shared credentials file. The session and its
// profile take the profile's name. A profile without a region gets the
// default region from Settings; STS needs one for the MFA session token.
func (s *Service) IAMUserFromProfile(profile string) (AddIAMUserInput, error) {
	for _, k := range awsconfig.IAMUserKeys(s.AWSConfigPath) {
		if k.Profile != profile {
			continue
		}
		in := AddIAMUserInput{Name: profile, Region: k.Region, MFADevice: k.MFADevice, Profile: profile, Key: aws.AccessKey{AccessKeyID: k.AccessKeyID, SecretAccessKey: k.SecretAccessKey}}
		if in.Region == "" {
			w, err := s.Load()
			if err != nil {
				return AddIAMUserInput{}, err
			}
			in.Region = w.EffectiveSettings().DefaultRegion
		}
		return in, nil
	}
	return AddIAMUserInput{}, fmt.Errorf("no access key for profile %q in %s", profile, awsconfig.Display(awsconfig.CredentialsPath(s.AWSConfigPath)))
}

// ImportIAMUser creates an IAM user session from the access key of a profile
// in the shared credentials file. The key stays in the file; the caller
// removes it, and rolle then serves the profile.
func (s *Service) ImportIAMUser(profile string) (core.Session, error) {
	in, err := s.IAMUserFromProfile(profile)
	if err != nil {
		return core.Session{}, err
	}
	return s.AddIAMUser(in)
}

// ImportResult describes an imported Identity Center portal.
type ImportResult struct {
	Integration core.Integration `json:"integration"`
	// LoggedIn is true when rolle reuses a valid AWS CLI token and discovers roles.
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
		// The CLI token is not usable. Drop it so the caller runs a login.
		_ = s.SSOLogout(in.ID)
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
	if err := checkAlias(w, alias, ""); err != nil {
		return core.Integration{}, err
	}
	in := core.Integration{ID: newID(), Alias: alias, Cloud: core.CloudAzure, Azure: &core.AzureIntegration{TenantID: tenantID}}
	w.Integrations = append(w.Integrations, in)
	return in, s.Save(w)
}

func (s *Service) azureAuth(in core.Integration) *azure.Auth {
	return &azure.Auth{Integration: in, Secrets: s.Secrets}
}

// azureIntegration resolves ref to an Entra ID tenant.
func azureIntegration(w *core.Workspace, ref string) (*core.Integration, error) {
	in, err := FindIntegration(w, ref)
	if err != nil {
		return nil, err
	}
	if in.Azure == nil {
		return nil, fmt.Errorf("integration %q is not an Azure tenant", in.Alias)
	}
	return in, nil
}

// gcpIntegration resolves ref to a Google Cloud account.
func gcpIntegration(w *core.Workspace, ref string) (*core.Integration, error) {
	in, err := FindIntegration(w, ref)
	if err != nil {
		return nil, err
	}
	if in.GCP == nil {
		return nil, fmt.Errorf("integration %q is not a Google Cloud account", in.Alias)
	}
	return in, nil
}

// forgetIntegration deletes the tokens an integration holds in the secret store.
func (s *Service) forgetIntegration(in core.Integration) error {
	switch {
	case in.AWSSSO != nil:
		return s.sso(in).Logout()
	case in.Azure != nil:
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return s.azureAuth(in).Logout(ctx)
	}
	return nil
}

// AzureLogin signs in through the browser, records the account, and discovers subscriptions.
func (s *Service) AzureLogin(ctx context.Context, ref string) ([]core.Session, error) {
	w, err := s.Load()
	if err != nil {
		return nil, err
	}
	in, err := azureIntegration(w, ref)
	if err != nil {
		return nil, err
	}
	account, err := s.azureAuth(*in).Login(ctx)
	if err != nil {
		return nil, err
	}
	// The browser sign-in can take minutes. Record the account in a fresh
	// copy of the workspace so a change made in the meantime is not lost.
	if w, err = s.Load(); err != nil {
		return nil, err
	}
	if in, err = azureIntegration(w, ref); err != nil {
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
	in, err := azureIntegration(w, ref)
	if err != nil {
		return nil, err
	}
	return s.azureAuth(*in).StartDeviceLogin(ctx)
}

// FinishAzureLogin records the account after a device-code login and syncs.
func (s *Service) FinishAzureLogin(ctx context.Context, ref, account string) ([]core.Session, error) {
	w, err := s.Load()
	if err != nil {
		return nil, err
	}
	in, err := azureIntegration(w, ref)
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
	in, err := azureIntegration(w, ref)
	if err != nil {
		return err
	}
	if err := s.azureAuth(*in).Logout(ctx); err != nil {
		return err
	}
	in.Azure.Account = ""
	// Cleanup failures are reported after the save, as in RemoveIntegration.
	var cleanup []error
	for i := range w.Sessions {
		if w.Sessions[i].IntegrationID == in.ID {
			if err := s.deactivate(&w.Sessions[i]); err != nil {
				cleanup = append(cleanup, err)
			}
		}
	}
	if err := s.Save(w); err != nil {
		return err
	}
	if len(cleanup) > 0 {
		return fmt.Errorf("%s signed out, but cleanup failed: %w", in.Alias, errors.Join(cleanup...))
	}
	return nil
}

// SyncAzure discovers subscriptions and adds a session per new one.
func (s *Service) SyncAzure(ctx context.Context, ref string) ([]core.Session, error) {
	w, err := s.Load()
	if err != nil {
		return nil, err
	}
	in, err := azureIntegration(w, ref)
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
	// Apply the result to a fresh copy of the workspace so a change made
	// during the network calls is not lost.
	if w, err = s.Load(); err != nil {
		return nil, err
	}
	if in, err = azureIntegration(w, ref); err != nil {
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
		if sess.IntegrationID == in.ID && sess.Kind == core.KindAzure && (sess.Azure == nil || !seen[sess.Azure.SubscriptionID]) {
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
	if err := checkAlias(w, alias, ""); err != nil {
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
	if _, err := gcpIntegration(w, ref); err != nil {
		return nil, err
	}
	tok, err := gcp.SourceToken(ctx)
	if err != nil {
		return nil, err
	}
	acct, _ := gcp.DetectAccount(ctx)
	projects, err := gcp.ListProjects(ctx, nil, tok.AccessToken)
	if err != nil {
		return nil, err
	}
	// Apply the result to a fresh copy of the workspace so a change made
	// during the network calls is not lost.
	if w, err = s.Load(); err != nil {
		return nil, err
	}
	in, err := gcpIntegration(w, ref)
	if err != nil {
		return nil, err
	}
	if acct.Email != "" {
		in.GCP.Account = acct.Email
	}
	var added []core.Session
	for _, p := range projects {
		if hasGCPProject(w, in.ID, p.ID) {
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

// hasGCPProject reports whether the plain project session already exists.
func hasGCPProject(w *core.Workspace, integrationID, projectID string) bool {
	for _, sess := range w.Sessions {
		if sess.IntegrationID == integrationID && sess.GCP != nil && sess.GCP.ProjectID == projectID && sess.GCP.ServiceAccount == "" {
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
	if err := checkSessionName(w, in.Name, ""); err != nil {
		return core.Session{}, err
	}
	if in.ProjectID == "" || in.ServiceAccount == "" {
		return core.Session{}, errors.New("project ID and service account are required")
	}
	integ, err := gcpIntegration(w, in.IntegrationRef)
	if err != nil {
		return core.Session{}, err
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

// writeCloudFiles creates the files other tools read for a session: the AWS
// profile, or the impersonated ADC file for a GCP service account session.
func (s *Service) writeCloudFiles(sess *core.Session) error {
	switch {
	case sess.Kind.Cloud() == core.CloudAWS:
		return awsconfig.Write(s.AWSConfigPath, awsconfig.Profile{
			Name: ProfileName(sess), Region: sess.Region, SessionID: sess.ID, Executable: s.Executable,
		})
	case sess.Kind == core.KindGCP && sess.GCP.ServiceAccount != "":
		return gcp.WriteImpersonatedADC(s.adcPath(sess), sess.GCP.ServiceAccount)
	}
	return nil
}

// removeCloudFiles deletes what writeCloudFiles created.
func (s *Service) removeCloudFiles(sess *core.Session) error {
	switch sess.Kind.Cloud() {
	case core.CloudAWS:
		return awsconfig.Remove(s.AWSConfigPath, ProfileName(sess), sess.ID)
	case core.CloudGCP:
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

// SessionEnv returns the environment variables for an active session with
// fresh credentials.
func (s *Service) SessionEnv(ctx context.Context, ref string) ([][2]string, error) {
	w, err := s.Load()
	if err != nil {
		return nil, err
	}
	sess, err := FindSession(w, ref)
	if err != nil {
		return nil, err
	}
	creds, err := s.credentials(ctx, w, sess)
	if err != nil {
		return nil, err
	}
	return s.EnvVars(sess, creds), nil
}

// terminalEnv returns what a shell needs for a session. AWS sessions use the
// profile so no secret is written; Azure and GCP export their short-lived
// tokens.
func (s *Service) terminalEnv(ctx context.Context, w *core.Workspace, sess *core.Session) ([][2]string, error) {
	if sess.Kind.Cloud() == core.CloudAWS {
		vars := [][2]string{{"AWS_PROFILE", ProfileName(sess)}}
		if sess.Region != "" {
			vars = append(vars, [2]string{"AWS_REGION", sess.Region}, [2]string{"AWS_DEFAULT_REGION", sess.Region})
		}
		return vars, nil
	}
	creds, err := s.credentials(ctx, w, sess)
	if err != nil {
		return nil, err
	}
	return s.EnvVars(sess, creds), nil
}

// OpenTerminal starts the session if needed and opens a terminal with its environment.
func (s *Service) OpenTerminal(ctx context.Context, ref string) error {
	w, err := s.Load()
	if err != nil {
		return err
	}
	sess, err := FindSession(w, ref)
	if err != nil {
		return err
	}
	if sess.Status != core.StatusActive {
		return fmt.Errorf("%s: %w", sess.Name, ErrSessionInactive)
	}
	env, err := s.terminalEnv(ctx, w, sess)
	if err != nil {
		return err
	}
	return terminal.Open(terminal.Options{
		Title: sess.Name,
		Env:   env,
		Dir:   filepath.Join(s.Cache.Dir, "launch"),
		App:   terminal.App(w.EffectiveSettings().Terminal),
	})
}

// LeappImportResult reports what the import recreated from a Leapp workspace.
type LeappImportResult struct {
	Sessions []core.Session `json:"sessions"`
	// Skipped lists the sessions the import did not recreate, and the reason.
	Skipped []string `json:"skipped"`
}

// ImportLeappSessions recreates IAM users and chained roles from the Leapp
// workspace. IAM user keys are copied from Leapp's keychain entries when the
// OS allows it; portals and tenants are imported through the normal paths.
func (s *Service) ImportLeappSessions(lw *discover.LeappWorkspace) (LeappImportResult, error) {
	var res LeappImportResult
	if lw == nil {
		return res, nil
	}
	w, err := s.Load()
	if err != nil {
		return res, err
	}
	names := map[string]bool{}
	for _, sess := range w.Sessions {
		names[sess.Name] = true
	}
	for _, u := range lw.IAMUsers {
		if names[u.Name] {
			res.Skipped = append(res.Skipped, u.Name+": already exists")
			continue
		}
		idKey, secretKey := discover.LeappKeychainKeys(u.ID)
		accessKeyID, err1 := keyringGet(discover.LeappKeychainService, idKey)
		secret, err2 := keyringGet(discover.LeappKeychainService, secretKey)
		if err1 != nil || err2 != nil || accessKeyID == "" || secret == "" {
			res.Skipped = append(res.Skipped, u.Name+": access key not found in the Leapp keychain")
			continue
		}
		sess, err := s.AddIAMUser(AddIAMUserInput{Name: u.Name, Region: u.Region, MFADevice: u.MFADevice, Profile: u.Profile, Key: aws.AccessKey{AccessKeyID: accessKeyID, SecretAccessKey: secret}})
		if err != nil {
			res.Skipped = append(res.Skipped, u.Name+": "+err.Error())
			continue
		}
		res.Sessions = append(res.Sessions, sess)
		names[u.Name] = true
	}
	for _, r := range lw.ChainedRoles {
		if names[r.Name] {
			res.Skipped = append(res.Skipped, r.Name+": already exists")
			continue
		}
		if !names[r.ParentName] {
			res.Skipped = append(res.Skipped, r.Name+": source session "+r.ParentName+" is not in rolle yet")
			continue
		}
		sess, err := s.AddAssumeRole(AddAssumeRoleInput{Name: r.Name, Region: r.Region, RoleARN: r.RoleARN, SourceRef: r.ParentName, Profile: r.Profile})
		if err != nil {
			res.Skipped = append(res.Skipped, r.Name+": "+err.Error())
			continue
		}
		res.Sessions = append(res.Sessions, sess)
		names[r.Name] = true
	}
	debug.Logf("discover", "leapp import: %d sessions, %d skipped", len(res.Sessions), len(res.Skipped))
	return res, nil
}

// keyringGet reads another application's keychain entry. Overridable in tests.
var keyringGet = func(service, key string) (string, error) { return keyring.Get(service, key) }

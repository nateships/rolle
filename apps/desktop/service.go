package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nateships/rolle/internal/app"
	"github.com/nateships/rolle/internal/aws"
	"github.com/nateships/rolle/internal/browser"
	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/discover"
	"github.com/nateships/rolle/internal/gcp"
	"github.com/nateships/rolle/internal/terminal"
	"github.com/nateships/rolle/internal/version"
)

// EventWorkspaceChanged is emitted after every workspace write so the UI can reload.
const EventWorkspaceChanged = "workspace:changed"

// RolleService exposes the application to the frontend.
type RolleService struct {
	svc *app.Service
	app *application.App

	mu      sync.Mutex
	pending map[string]*aws.DeviceAuthorization
}

// NewRolleService wires the service to the shared application layer.
func NewRolleService(svc *app.Service) *RolleService {
	return &RolleService{svc: svc, pending: map[string]*aws.DeviceAuthorization{}}
}

// ServiceStartup captures the running application for event emission.
func (r *RolleService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	r.app = application.Get()
	return nil
}

// ServiceName lets Wails and debug output identify this service.
func (r *RolleService) ServiceName() string { return "rolle" }

func ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 2*time.Minute)
}

// Workspace returns the workspace with session statuses reconciled.
func (r *RolleService) Workspace() (*core.Workspace, error) { return r.svc.Refresh() }

// CompleteOnboarding marks the walkthrough as done.
func (r *RolleService) CompleteOnboarding() error {
	w, err := r.svc.Load()
	if err != nil {
		return err
	}
	w.Onboarded = true
	return r.svc.Save(w)
}

// AddAWSSSO registers an IAM Identity Center portal.
func (r *RolleService) AddAWSSSO(alias, startURL, region string) (core.Integration, error) {
	return r.svc.AddAWSSSO(alias, startURL, region)
}

// DeviceLogin is what the user must do to approve a login. UserCode is empty
// for the browser flow.
type DeviceLogin struct {
	VerificationURI string `json:"verificationUri"`
	UserCode        string `json:"userCode"`
}

// StartSSOLogin begins the browser sign-in and opens the page. Call WaitSSOLogin next.
func (r *RolleService) StartSSOLogin(ref string) (DeviceLogin, error) {
	c, cancel := ctx()
	defer cancel()
	auth, err := r.svc.SSOLogin(c, ref)
	if err != nil {
		return DeviceLogin{}, err
	}
	r.mu.Lock()
	r.pending[ref] = auth
	r.mu.Unlock()
	_ = browser.Open(auth.VerificationURI)
	return DeviceLogin{VerificationURI: auth.VerificationURI, UserCode: auth.UserCode}, nil
}

// WaitSSOLogin blocks until the user approves, then discovers roles.
func (r *RolleService) WaitSSOLogin(ref string) ([]core.Session, error) {
	r.mu.Lock()
	auth := r.pending[ref]
	delete(r.pending, ref)
	r.mu.Unlock()
	if auth == nil {
		return nil, fmt.Errorf("no login in progress for %s", ref)
	}
	c, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	if err := auth.Wait(c); err != nil {
		return nil, err
	}
	return r.svc.FinishSSOLogin(c, ref)
}

// SSOLogout signs out of a portal.
func (r *RolleService) SSOLogout(ref string) error {
	return r.svc.SSOLogout(ref)
}

// SyncSSO rediscovers roles.
func (r *RolleService) SyncSSO(ref string) ([]core.Session, error) {
	c, cancel := ctx()
	defer cancel()
	return r.svc.SyncSSO(c, ref)
}

// RemoveIntegration deletes a portal and its sessions.
func (r *RolleService) RemoveIntegration(ref string) error {
	return r.svc.RemoveIntegration(ref)
}

// AddAssumeRole creates a role-chaining session.
func (r *RolleService) AddAssumeRole(in app.AddAssumeRoleInput) (core.Session, error) {
	return r.svc.AddAssumeRole(in)
}

// IAMUserInput is the frontend shape for a new IAM user session.
type IAMUserInput struct {
	Name            string `json:"name"`
	Region          string `json:"region"`
	AccessKeyID     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
	MFADevice       string `json:"mfaDevice"`
}

// AddIAMUser creates a session backed by an access key.
func (r *RolleService) AddIAMUser(in IAMUserInput) (core.Session, error) {
	return r.svc.AddIAMUser(app.AddIAMUserInput{
		Name: in.Name, Region: in.Region, MFADevice: in.MFADevice,
		Key: aws.AccessKey{AccessKeyID: in.AccessKeyID, SecretAccessKey: in.SecretAccessKey},
	})
}

// RemoveSession deletes a session.
func (r *RolleService) RemoveSession(ref string) error {
	return r.svc.RemoveSession(ref)
}

// Start activates a session. mfaCode may be empty.
func (r *RolleService) Start(ref, mfaCode string) (core.Credentials, error) {
	c, cancel := ctx()
	defer cancel()
	return r.svc.Start(c, ref, app.StartOptions{MFACode: mfaCode})
}

// Stop deactivates a session.
func (r *RolleService) Stop(ref string) error {
	return r.svc.Stop(ref)
}

// Credentials returns fresh credentials for an active session.
func (r *RolleService) Credentials(ref string) (core.Credentials, error) {
	c, cancel := ctx()
	defer cancel()
	return r.svc.Credentials(c, ref)
}

// ProfileName returns the AWS profile a session writes.
func (r *RolleService) ProfileName(ref string) (string, error) {
	w, err := r.svc.Load()
	if err != nil {
		return "", err
	}
	s, err := app.FindSession(w, ref)
	if err != nil {
		return "", err
	}
	return app.ProfileName(s), nil
}

// OpenConsole opens the AWS console for a session in the browser.
func (r *RolleService) OpenConsole(ref string) error {
	c, cancel := ctx()
	defer cancel()
	u, err := r.svc.ConsoleURLFor(c, ref)
	if err != nil {
		return err
	}
	return browser.Open(u)
}

// EnvText returns shell export lines for an active session.
func (r *RolleService) EnvText(ref string) (string, error) {
	c, cancel := ctx()
	defer cancel()
	env, err := r.svc.SessionEnv(c, ref)
	if err != nil {
		return "", err
	}
	return terminal.Exports(env, false), nil
}

// AddAzure registers an Entra ID tenant.
func (r *RolleService) AddAzure(alias, tenantID string) (core.Integration, error) {
	return r.svc.AddAzure(alias, tenantID)
}

// AzureLogin opens the browser sign-in, then discovers subscriptions.
func (r *RolleService) AzureLogin(ref string) ([]core.Session, error) {
	c, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	return r.svc.AzureLogin(c, ref)
}

// AzureLogout signs out of a tenant.
func (r *RolleService) AzureLogout(ref string) error {
	c, cancel := ctx()
	defer cancel()
	return r.svc.AzureLogout(c, ref)
}

// SyncAzure rediscovers subscriptions.
func (r *RolleService) SyncAzure(ref string) ([]core.Session, error) {
	c, cancel := ctx()
	defer cancel()
	return r.svc.SyncAzure(c, ref)
}

// GCPStatus reports whether gcloud Application Default Credentials exist and
// whether the gcloud CLI itself is available.
type GCPStatus struct {
	Ready        bool   `json:"ready"`
	Account      string `json:"account"`
	LoginCommand string `json:"loginCommand"`
	GCloudFound  bool   `json:"gcloudFound"`
	GCloudPath   string `json:"gcloudPath"`
	InstallURL   string `json:"installUrl"`
}

// GCPStatus checks for local gcloud credentials and the gcloud CLI.
func (r *RolleService) GCPStatus() GCPStatus {
	c, cancel := ctx()
	defer cancel()
	st := GCPStatus{LoginCommand: gcp.LoginCommand, InstallURL: gcp.InstallURL}
	if p, err := gcp.FindGCloud(); err == nil {
		st.GCloudFound, st.GCloudPath = true, p
	}
	if acct, err := gcp.DetectAccount(c); err == nil {
		st.Ready, st.Account = true, acct.Email
	}
	return st
}

// GCloudLogin runs the gcloud Application Default Credentials login, which
// opens the browser. Blocks until gcloud finishes.
func (r *RolleService) GCloudLogin() error {
	c, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	return gcp.GCloudLogin(c)
}

// AddGCP registers gcloud credentials and discovers projects.
func (r *RolleService) AddGCP(alias string) ([]core.Session, error) {
	c, cancel := ctx()
	defer cancel()
	_, added, err := r.svc.AddGCP(c, alias)
	return added, err
}

// SyncGCP rediscovers projects.
func (r *RolleService) SyncGCP(ref string) ([]core.Session, error) {
	c, cancel := ctx()
	defer cancel()
	return r.svc.SyncGCP(c, ref)
}

// AddGCPImpersonation creates a service account impersonation session.
func (r *RolleService) AddGCPImpersonation(in app.AddGCPImpersonationInput) (core.Session, error) {
	return r.svc.AddGCPImpersonation(in)
}

// Settings returns the effective user preferences.
func (r *RolleService) Settings() (core.Settings, error) { return r.svc.Settings() }

// UpdateSettings stores preferences.
func (r *RolleService) UpdateSettings(in core.Settings) (core.Settings, error) {
	return r.svc.UpdateSettings(in)
}

// AppInfo describes this build and where it keeps its files.
type AppInfo struct {
	Version       string `json:"version"`
	WorkspacePath string `json:"workspacePath"`
	CacheDir      string `json:"cacheDir"`
	AWSConfigPath string `json:"awsConfigPath"`
}

// Info returns version and file locations for the settings screen.
func (r *RolleService) Info() AppInfo {
	return AppInfo{Version: version.Version, WorkspacePath: r.svc.WorkspacePath, CacheDir: r.svc.Cache.Dir, AWSConfigPath: r.svc.AWSConfigPath}
}

// ReplayOnboarding shows the walkthrough again without removing sessions.
func (r *RolleService) ReplayOnboarding() error {
	return r.svc.ReplayOnboarding()
}

// Reset removes every session, integration, secret, cached credential, and
// Rolle-owned AWS profile. The UI confirms before calling this.
func (r *RolleService) Reset() error {
	return r.svc.ResetAll()
}

// Discover reports identities other tools already configured on this machine.
func (r *RolleService) Discover() discover.Result {
	c, cancel := ctx()
	defer cancel()
	return r.svc.Discover(c)
}

// ImportLeappSessions recreates IAM users and chained roles from a Leapp workspace.
func (r *RolleService) ImportLeappSessions() (app.LeappImportResult, error) {
	lw, err := discover.ReadLeapp()
	if err != nil {
		return app.LeappImportResult{}, err
	}
	return r.svc.ImportLeappSessions(lw)
}

// ImportAWSSSO registers a portal from the AWS CLI config, reusing its token when valid.
func (r *RolleService) ImportAWSSSO(alias, startURL, region string) (app.ImportResult, error) {
	c, cancel := ctx()
	defer cancel()
	return r.svc.ImportAWSSSO(c, alias, startURL, region)
}

// RenameIntegration changes an integration's display name.
func (r *RolleService) RenameIntegration(ref, alias string) error {
	return r.svc.RenameIntegration(ref, alias)
}

// SetFavorite pins or unpins a session.
func (r *RolleService) SetFavorite(ref string, favorite bool) error {
	return r.svc.SetFavorite(ref, favorite)
}

// SetRegion changes an AWS session's region.
func (r *RolleService) SetRegion(ref, region string) error { return r.svc.SetRegion(ref, region) }

// SetProfile sets the AWS profile name for a session. Empty restores the default.
func (r *RolleService) SetProfile(ref, profile string) error { return r.svc.SetProfile(ref, profile) }

// RenameSession changes a session's name.
func (r *RolleService) RenameSession(ref, name string) error {
	return r.svc.RenameSession(ref, name)
}

// DevMode reports whether in-app dev tools are compiled in.
func (r *RolleService) DevMode() bool { return devMode }

// OpenTerminal opens the user's terminal with the session's environment ready.
func (r *RolleService) OpenTerminal(ref string) error {
	c, cancel := ctx()
	defer cancel()
	return r.svc.OpenTerminal(c, ref)
}

// OpenURL opens a link in the default browser.
func (r *RolleService) OpenURL(u string) error { return browser.Open(u) }

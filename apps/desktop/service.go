package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nateships/rolle/internal/app"
	"github.com/nateships/rolle/internal/aws"
	"github.com/nateships/rolle/internal/browser"
	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/gcp"
)

// EventWorkspaceChanged is emitted after any mutation so the UI can reload.
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

func (r *RolleService) changed() {
	if r.app != nil {
		r.app.Event.Emit(EventWorkspaceChanged, struct{}{})
	}
}

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
	if err := r.svc.Save(w); err != nil {
		return err
	}
	r.changed()
	return nil
}

// AddAWSSSO registers an IAM Identity Center portal.
func (r *RolleService) AddAWSSSO(alias, startURL, region string) (core.Integration, error) {
	in, err := r.svc.AddAWSSSO(alias, startURL, region)
	if err == nil {
		r.changed()
	}
	return in, err
}

// DeviceLogin is what the user must do to approve a login.
type DeviceLogin struct {
	VerificationURI string `json:"verificationUri"`
	UserCode        string `json:"userCode"`
}

// StartSSOLogin begins the device flow and opens the browser. Call WaitSSOLogin next.
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
	added, err := r.svc.FinishSSOLogin(c, ref)
	r.changed()
	return added, err
}

// SSOLogout signs out of a portal.
func (r *RolleService) SSOLogout(ref string) error {
	err := r.svc.SSOLogout(ref)
	r.changed()
	return err
}

// SyncSSO rediscovers roles.
func (r *RolleService) SyncSSO(ref string) ([]core.Session, error) {
	c, cancel := ctx()
	defer cancel()
	added, err := r.svc.SyncSSO(c, ref)
	r.changed()
	return added, err
}

// RemoveIntegration deletes a portal and its sessions.
func (r *RolleService) RemoveIntegration(ref string) error {
	err := r.svc.RemoveIntegration(ref)
	r.changed()
	return err
}

// AddAssumeRole creates a role-chaining session.
func (r *RolleService) AddAssumeRole(in app.AddAssumeRoleInput) (core.Session, error) {
	s, err := r.svc.AddAssumeRole(in)
	r.changed()
	return s, err
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
	s, err := r.svc.AddIAMUser(app.AddIAMUserInput{
		Name: in.Name, Region: in.Region, MFADevice: in.MFADevice,
		Key: aws.AccessKey{AccessKeyID: in.AccessKeyID, SecretAccessKey: in.SecretAccessKey},
	})
	r.changed()
	return s, err
}

// RemoveSession deletes a session.
func (r *RolleService) RemoveSession(ref string) error {
	err := r.svc.RemoveSession(ref)
	r.changed()
	return err
}

// Start activates a session. mfaCode may be empty.
func (r *RolleService) Start(ref, mfaCode string) (core.Credentials, error) {
	c, cancel := ctx()
	defer cancel()
	creds, err := r.svc.Start(c, ref, app.StartOptions{MFACode: mfaCode})
	r.changed()
	return creds, err
}

// Stop deactivates a session.
func (r *RolleService) Stop(ref string) error {
	err := r.svc.Stop(ref)
	r.changed()
	return err
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
	w, err := r.svc.Load()
	if err != nil {
		return "", err
	}
	sess, err := app.FindSession(w, ref)
	if err != nil {
		return "", err
	}
	creds, err := r.svc.Credentials(c, ref)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, kv := range r.svc.EnvVars(sess, creds) {
		if kv[1] != "" {
			fmt.Fprintf(&b, "export %s=%q\n", kv[0], kv[1])
		}
	}
	return b.String(), nil
}

// AddAzure registers an Entra ID tenant.
func (r *RolleService) AddAzure(alias, tenantID string) (core.Integration, error) {
	in, err := r.svc.AddAzure(alias, tenantID)
	r.changed()
	return in, err
}

// AzureLogin opens the browser sign-in, then discovers subscriptions.
func (r *RolleService) AzureLogin(ref string) ([]core.Session, error) {
	c, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	added, err := r.svc.AzureLogin(c, ref)
	r.changed()
	return added, err
}

// AzureLogout signs out of a tenant.
func (r *RolleService) AzureLogout(ref string) error {
	c, cancel := ctx()
	defer cancel()
	err := r.svc.AzureLogout(c, ref)
	r.changed()
	return err
}

// SyncAzure rediscovers subscriptions.
func (r *RolleService) SyncAzure(ref string) ([]core.Session, error) {
	c, cancel := ctx()
	defer cancel()
	added, err := r.svc.SyncAzure(c, ref)
	r.changed()
	return added, err
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
	r.changed()
	return added, err
}

// SyncGCP rediscovers projects.
func (r *RolleService) SyncGCP(ref string) ([]core.Session, error) {
	c, cancel := ctx()
	defer cancel()
	added, err := r.svc.SyncGCP(c, ref)
	r.changed()
	return added, err
}

// AddGCPImpersonation creates a service account impersonation session.
func (r *RolleService) AddGCPImpersonation(in app.AddGCPImpersonationInput) (core.Session, error) {
	s, err := r.svc.AddGCPImpersonation(in)
	r.changed()
	return s, err
}

// OpenURL opens a link in the default browser.
func (r *RolleService) OpenURL(u string) error { return browser.Open(u) }

// Package app orchestrates the workspace, secret store, credential cache, and
// cloud providers. The CLI and desktop app call into it.
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nateships/rolle/internal/aws"
	"github.com/nateships/rolle/internal/awsconfig"
	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/credcache"
	"github.com/nateships/rolle/internal/debug"
	"github.com/nateships/rolle/internal/netcfg"
	"github.com/nateships/rolle/internal/secrets"
	"github.com/nateships/rolle/internal/workspace"
)

// Service is the application façade.
type Service struct {
	WorkspacePath string
	AWSConfigPath string
	// Executable is the rolle CLI path written into credential_process.
	Executable string
	Secrets    secrets.Store
	Cache      *credcache.Cache
	Now        func() time.Time
	// OnChange runs after every workspace write. Nil means no listener.
	OnChange func()
}

// Default builds a Service with production paths.
func Default() (*Service, error) {
	awsPath, err := awsconfig.DefaultPath()
	if err != nil {
		return nil, err
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	s := &Service{
		WorkspacePath: workspace.DefaultPath(),
		AWSConfigPath: awsPath,
		Executable:    exe,
		Secrets:       secrets.NewKeychain(),
		Cache:         credcache.Default(),
	}
	// A bad proxy or bundle must not stop the app; it is reported when edited.
	if st, err := s.Settings(); err == nil {
		if err := netcfg.Apply(st); err != nil {
			debug.Logf("network", "%v", err)
		}
	}
	return s, nil
}

// Load reads the workspace.
func (s *Service) Load() (*core.Workspace, error) { return workspace.Load(s.WorkspacePath) }

// Save writes the workspace and notifies OnChange.
func (s *Service) Save(w *core.Workspace) error {
	if err := workspace.Save(s.WorkspacePath, w); err != nil {
		return err
	}
	s.notify()
	return nil
}

func (s *Service) notify() {
	if s.OnChange != nil {
		s.OnChange()
	}
}

// ErrAmbiguous is returned when a name or ID prefix matches more than one item.
var ErrAmbiguous = errors.New("ambiguous reference")

// FindSession resolves a session by exact ID, exact name, or unique ID prefix.
func FindSession(w *core.Workspace, ref string) (*core.Session, error) {
	var matches []*core.Session
	for i := range w.Sessions {
		sess := &w.Sessions[i]
		if sess.ID == ref || sess.Name == ref {
			return sess, nil
		}
		if strings.HasPrefix(sess.ID, ref) {
			matches = append(matches, sess)
		}
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("session %q: %w", ref, core.ErrNotFound)
	case 1:
		return matches[0], nil
	}
	return nil, fmt.Errorf("session %q: %w", ref, ErrAmbiguous)
}

// FindIntegration resolves an integration by exact ID, alias, or unique ID prefix.
func FindIntegration(w *core.Workspace, ref string) (*core.Integration, error) {
	var matches []*core.Integration
	for i := range w.Integrations {
		in := &w.Integrations[i]
		if in.ID == ref || in.Alias == ref {
			return in, nil
		}
		if strings.HasPrefix(in.ID, ref) {
			matches = append(matches, in)
		}
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("integration %q: %w", ref, core.ErrNotFound)
	case 1:
		return matches[0], nil
	}
	return nil, fmt.Errorf("integration %q: %w", ref, ErrAmbiguous)
}

func newID() string { return uuid.NewString() }

// ssoIntegration resolves ref to an Identity Center portal.
func ssoIntegration(w *core.Workspace, ref string) (*core.Integration, error) {
	in, err := FindIntegration(w, ref)
	if err != nil {
		return nil, err
	}
	if in.AWSSSO == nil {
		return nil, fmt.Errorf("integration %q is not an AWS IAM Identity Center portal", in.Alias)
	}
	return in, nil
}

// AddAWSSSO registers an IAM Identity Center portal.
func (s *Service) AddAWSSSO(alias, startURL, region string) (core.Integration, error) {
	w, err := s.Load()
	if err != nil {
		return core.Integration{}, err
	}
	in := core.Integration{
		ID:     newID(),
		Alias:  alias,
		Cloud:  core.CloudAWS,
		AWSSSO: &core.AWSSSOIntegration{StartURL: startURL, Region: region},
	}
	w.Integrations = append(w.Integrations, in)
	return in, s.Save(w)
}

// RemoveIntegration deletes an integration, its token, and its sessions.
func (s *Service) RemoveIntegration(ref string) error {
	w, err := s.Load()
	if err != nil {
		return err
	}
	in, err := FindIntegration(w, ref)
	if err != nil {
		return err
	}
	s.forgetIntegration(*in)
	for _, sess := range w.Sessions {
		if sess.IntegrationID == in.ID {
			_ = s.deactivate(&sess)
		}
	}
	if err := w.RemoveIntegration(in.ID); err != nil {
		return err
	}
	return s.Save(w)
}

func (s *Service) sso(in core.Integration) *aws.SSO {
	return &aws.SSO{Integration: in, Secrets: s.Secrets, Now: s.Now}
}

// SSOLogin starts a browser sign-in for an Identity Center integration. The
// returned authorization must be completed with Wait; FinishSSOLogin then
// records the token expiry and discovers roles.
func (s *Service) SSOLogin(ctx context.Context, ref string) (*aws.DeviceAuthorization, error) {
	w, err := s.Load()
	if err != nil {
		return nil, err
	}
	in, err := ssoIntegration(w, ref)
	if err != nil {
		return nil, err
	}
	return s.sso(*in).StartLogin(ctx)
}

// SSODeviceLogin starts the device code flow, for terminals without a browser.
func (s *Service) SSODeviceLogin(ctx context.Context, ref string) (*aws.DeviceAuthorization, error) {
	w, err := s.Load()
	if err != nil {
		return nil, err
	}
	in, err := ssoIntegration(w, ref)
	if err != nil {
		return nil, err
	}
	return s.sso(*in).StartDeviceLogin(ctx)
}

// FinishSSOLogin records the token expiry and syncs roles after a login.
func (s *Service) FinishSSOLogin(ctx context.Context, ref string) ([]core.Session, error) {
	w, err := s.Load()
	if err != nil {
		return nil, err
	}
	in, err := ssoIntegration(w, ref)
	if err != nil {
		return nil, err
	}
	in.AWSSSO.TokenExpires = s.sso(*in).TokenExpiry()
	if err := s.Save(w); err != nil {
		return nil, err
	}
	return s.SyncSSO(ctx, ref)
}

// SSOLogout drops the token and deactivates the integration's sessions.
func (s *Service) SSOLogout(ref string) error {
	w, err := s.Load()
	if err != nil {
		return err
	}
	in, err := ssoIntegration(w, ref)
	if err != nil {
		return err
	}
	if err := s.sso(*in).Logout(); err != nil {
		return err
	}
	in.AWSSSO.TokenExpires = nil
	for i := range w.Sessions {
		if w.Sessions[i].IntegrationID == in.ID {
			_ = s.deactivate(&w.Sessions[i])
		}
	}
	return s.Save(w)
}

// SyncSSO discovers accounts and roles and adds a session for each new role.
// Existing sessions are kept. Sessions whose role disappeared are removed.
func (s *Service) SyncSSO(ctx context.Context, ref string) ([]core.Session, error) {
	w, err := s.Load()
	if err != nil {
		return nil, err
	}
	in, err := ssoIntegration(w, ref)
	if err != nil {
		return nil, err
	}
	sso := s.sso(*in)
	accounts, err := sso.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var added []core.Session
	for _, acct := range accounts {
		roles, err := sso.ListRoles(ctx, acct.ID)
		if err != nil {
			return nil, err
		}
		for _, role := range roles {
			key := acct.ID + "/" + role.Name
			seen[key] = true
			if hasSSORole(w, in.ID, acct.ID, role.Name) {
				continue
			}
			sess := core.Session{
				ID:            newID(),
				Name:          acct.Name + "/" + role.Name,
				Kind:          core.KindAWSSSORole,
				Region:        in.AWSSSO.Region,
				IntegrationID: in.ID,
				Status:        core.StatusInactive,
				AWS:           &core.AWSSession{AccountID: acct.ID, RoleName: role.Name},
			}
			w.Sessions = append(w.Sessions, sess)
			added = append(added, sess)
		}
	}
	kept := w.Sessions[:0]
	for _, sess := range w.Sessions {
		if sess.IntegrationID == in.ID && sess.Kind == core.KindAWSSSORole && !seen[sess.AWS.AccountID+"/"+sess.AWS.RoleName] {
			_ = s.deactivate(&sess)
			continue
		}
		kept = append(kept, sess)
	}
	w.Sessions = kept
	return added, s.Save(w)
}

func hasSSORole(w *core.Workspace, integrationID, accountID, role string) bool {
	for _, sess := range w.Sessions {
		if sess.IntegrationID == integrationID && sess.AWS != nil && sess.AWS.AccountID == accountID && sess.AWS.RoleName == role {
			return true
		}
	}
	return false
}

// AddAssumeRoleInput describes a new AssumeRole session.
type AddAssumeRoleInput struct {
	Name       string `json:"name"`
	Region     string `json:"region"`
	RoleARN    string `json:"roleArn"`
	SourceRef  string `json:"sourceRef"`
	ExternalID string `json:"externalId"`
	Profile    string `json:"profile"`
}

// AddAssumeRole creates a session that assumes a role from another session.
func (s *Service) AddAssumeRole(in AddAssumeRoleInput) (core.Session, error) {
	if strings.TrimSpace(in.Name) == "" {
		return core.Session{}, errors.New("name cannot be empty")
	}
	if !strings.HasPrefix(in.RoleARN, "arn:") {
		return core.Session{}, fmt.Errorf("role ARN %q is not an ARN", in.RoleARN)
	}
	w, err := s.Load()
	if err != nil {
		return core.Session{}, err
	}
	src, err := FindSession(w, in.SourceRef)
	if err != nil {
		return core.Session{}, err
	}
	if src.Kind.Cloud() != core.CloudAWS {
		return core.Session{}, fmt.Errorf("source session %q is not an AWS session", src.Name)
	}
	sess := core.Session{
		ID:     newID(),
		Name:   in.Name,
		Kind:   core.KindAWSAssumeRole,
		Region: in.Region,
		Status: core.StatusInactive,
		AWS:    &core.AWSSession{RoleARN: in.RoleARN, SourceSessionID: src.ID, ExternalID: in.ExternalID, Profile: in.Profile},
	}
	w.Sessions = append(w.Sessions, sess)
	return sess, s.Save(w)
}

// AddIAMUserInput describes a new IAM user session.
type AddIAMUserInput struct {
	Name, Region, MFADevice, Profile string
	Key                              aws.AccessKey
}

// AddIAMUser creates a session backed by a long-lived access key.
func (s *Service) AddIAMUser(in AddIAMUserInput) (core.Session, error) {
	w, err := s.Load()
	if err != nil {
		return core.Session{}, err
	}
	sess := core.Session{
		ID:     newID(),
		Name:   in.Name,
		Kind:   core.KindAWSIAMUser,
		Region: in.Region,
		Status: core.StatusInactive,
		AWS:    &core.AWSSession{MFADevice: in.MFADevice, Profile: in.Profile},
	}
	if err := aws.StoreAccessKey(s.Secrets, sess.ID, in.Key); err != nil {
		return core.Session{}, err
	}
	w.Sessions = append(w.Sessions, sess)
	return sess, s.Save(w)
}

// RemoveSession deletes a session and everything cached for it.
func (s *Service) RemoveSession(ref string) error {
	w, err := s.Load()
	if err != nil {
		return err
	}
	sess, err := FindSession(w, ref)
	if err != nil {
		return err
	}
	for _, other := range w.Sessions {
		if other.AWS != nil && other.AWS.SourceSessionID == sess.ID {
			return fmt.Errorf("session %q is the source of %q; remove that first", sess.Name, other.Name)
		}
	}
	_ = s.deactivate(sess)
	if sess.Kind == core.KindAWSIAMUser {
		_ = aws.DeleteAccessKey(s.Secrets, sess.ID)
	}
	if err := w.RemoveSession(sess.ID); err != nil {
		return err
	}
	return s.Save(w)
}

// ProfileName returns the AWS profile a session writes.
// ProfileName returns the AWS profile a session writes. Sessions share the
// "default" profile unless one is set, so `aws` works without --profile and
// only one such session is active at a time.
func ProfileName(sess *core.Session) string {
	if sess.AWS != nil && sess.AWS.Profile != "" {
		return sess.AWS.Profile
	}
	return "default"
}

func sanitizeProfile(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// StartOptions tune Start.
type StartOptions struct {
	// MFACode is required for IAM users and roles that enforce MFA.
	MFACode string
}

// Start fetches credentials for a session, caches them, marks the session
// active, and writes its AWS profile.
func (s *Service) Start(ctx context.Context, ref string, opts StartOptions) (core.Credentials, error) {
	w, err := s.Load()
	if err != nil {
		return core.Credentials{}, err
	}
	sess, err := FindSession(w, ref)
	if err != nil {
		return core.Credentials{}, err
	}
	debug.Logf("session", "start %s (%s)", sess.Name, sess.Kind)
	creds, err := s.fetch(ctx, w, sess, opts.MFACode)
	if err != nil {
		debug.Logf("session", "start %s failed: %v", sess.Name, err)
		return core.Credentials{}, err
	}
	if err := s.Cache.Put(sess.ID, creds); err != nil {
		return core.Credentials{}, err
	}
	if sess.Kind.Cloud() == core.CloudAWS {
		// One active session per profile: the newest start takes it over.
		for i := range w.Sessions {
			other := &w.Sessions[i]
			if other.ID != sess.ID && other.Status == core.StatusActive && other.Kind.Cloud() == core.CloudAWS && ProfileName(other) == ProfileName(sess) {
				debug.Logf("session", "stop %s: profile %s taken over by %s", other.Name, ProfileName(other), sess.Name)
				if err := s.deactivate(other); err != nil {
					return core.Credentials{}, err
				}
			}
		}
	}
	sess.Status = core.StatusActive
	sess.Expires = creds.Expiration
	if err := s.writeCloudFiles(sess); err != nil {
		return core.Credentials{}, err
	}
	return creds, s.Save(w)
}

// Stop drops cached credentials, removes the AWS profile, and marks the session inactive.
func (s *Service) Stop(ref string) error {
	w, err := s.Load()
	if err != nil {
		return err
	}
	sess, err := FindSession(w, ref)
	if err != nil {
		return err
	}
	if err := s.deactivate(sess); err != nil {
		return err
	}
	return s.Save(w)
}

func (s *Service) deactivate(sess *core.Session) error {
	if err := s.Cache.Delete(sess.ID); err != nil {
		return err
	}
	if err := s.removeCloudFiles(sess); err != nil {
		return err
	}
	sess.Status = core.StatusInactive
	sess.Expires = nil
	return nil
}

// ErrSessionInactive is returned when credentials are requested for a stopped session.
var ErrSessionInactive = errors.New("session is not started")

// Credentials returns fresh credentials for an active session, refreshing
// silently when the cache is stale. This backs credential_process.
func (s *Service) Credentials(ctx context.Context, ref string) (core.Credentials, error) {
	w, err := s.Load()
	if err != nil {
		return core.Credentials{}, err
	}
	sess, err := FindSession(w, ref)
	if err != nil {
		return core.Credentials{}, err
	}
	return s.credentials(ctx, w, sess)
}

func (s *Service) credentials(ctx context.Context, w *core.Workspace, sess *core.Session) (core.Credentials, error) {
	if sess.Status != core.StatusActive {
		return core.Credentials{}, fmt.Errorf("%s: %w", sess.Name, ErrSessionInactive)
	}
	if creds, err := s.Cache.Get(sess.ID); err == nil {
		debug.Logf("creds", "%s served from cache", sess.Name)
		return creds, nil
	}
	debug.Logf("creds", "%s cache miss, refreshing", sess.Name)
	creds, err := s.fetch(ctx, w, sess, "")
	if err != nil {
		debug.Logf("creds", "%s refresh failed: %v", sess.Name, err)
		return core.Credentials{}, err
	}
	if err := s.Cache.Put(sess.ID, creds); err != nil {
		return core.Credentials{}, err
	}
	sess.Expires = creds.Expiration
	return creds, s.Save(w)
}

// fetch obtains credentials from the provider behind a session.
func (s *Service) fetch(ctx context.Context, w *core.Workspace, sess *core.Session, mfaCode string) (core.Credentials, error) {
	switch sess.Kind {
	case core.KindAWSSSORole:
		in, err := w.Integration(sess.IntegrationID)
		if err != nil {
			return core.Credentials{}, fmt.Errorf("integration for %s: %w", sess.Name, err)
		}
		return s.sso(*in).RoleCredentials(ctx, sess.AWS.AccountID, sess.AWS.RoleName)
	case core.KindAWSAssumeRole:
		src, err := w.Session(sess.AWS.SourceSessionID)
		if err != nil {
			return core.Credentials{}, fmt.Errorf("source session for %s: %w", sess.Name, err)
		}
		source, err := s.credentials(ctx, w, src)
		if err != nil {
			return core.Credentials{}, err
		}
		duration := time.Duration(w.EffectiveSettings().AssumeRoleMinutes) * time.Minute
		// STS caps role chaining at one hour.
		if src.Kind != core.KindAWSIAMUser && duration > time.Hour {
			duration = time.Hour
		}
		return aws.AssumeRole(ctx, aws.AssumeRoleInput{
			Source:      source,
			Region:      sess.Region,
			RoleARN:     sess.AWS.RoleARN,
			Duration:    duration,
			SessionName: "rolle-" + sanitizeProfile(sess.Name),
			ExternalID:  sess.AWS.ExternalID,
			MFADevice:   sess.AWS.MFADevice,
			MFACode:     mfaCode,
		})
	case core.KindAWSIAMUser:
		key, err := aws.LoadAccessKey(s.Secrets, sess.ID)
		if err != nil {
			return core.Credentials{}, err
		}
		if sess.AWS.MFADevice != "" && mfaCode == "" {
			if creds, err := s.Cache.Get(sess.ID); err == nil {
				return creds, nil
			}
			return core.Credentials{}, fmt.Errorf("%s: MFA code required", sess.Name)
		}
		return aws.IAMUserCredentials(ctx, aws.IAMUserInput{Key: key, Region: sess.Region, MFADevice: sess.AWS.MFADevice, MFACode: mfaCode})
	}
	return s.fetchCloud(ctx, w, sess)
}

// ConsoleURL returns a federated AWS console sign-in link for an active session.
func (s *Service) ConsoleURL(ctx context.Context, ref string) (string, error) {
	w, err := s.Load()
	if err != nil {
		return "", err
	}
	sess, err := FindSession(w, ref)
	if err != nil {
		return "", err
	}
	if sess.Kind.Cloud() != core.CloudAWS {
		return "", fmt.Errorf("console links are only available for AWS sessions")
	}
	if sess.Kind == core.KindAWSIAMUser {
		return "", fmt.Errorf("%s: console sign-in needs a role; add an assume-role session", sess.Name)
	}
	creds, err := s.credentials(ctx, w, sess)
	if err != nil {
		return "", err
	}
	return aws.ConsoleURL(ctx, nil, creds, sess.Region)
}

// Refresh reconciles session status with the credential cache. Active
// sessions whose credentials expired are renewed when the provider allows a
// silent refresh (Identity Center roles, role chains, Azure, GCP). Sessions
// that cannot be renewed without user input are marked inactive.
func (s *Service) Refresh() (*core.Workspace, error) {
	w, err := s.Load()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// Renewals take network time. Collect the results first, then apply them
	// to a fresh copy of the workspace so a concurrent change is not lost.
	renewed := map[string]*time.Time{}
	expired := map[string]bool{}
	for i := range w.Sessions {
		sess := &w.Sessions[i]
		if sess.Status != core.StatusActive {
			continue
		}
		if _, err := s.Cache.Get(sess.ID); err == nil {
			continue
		}
		if !s.renewable(sess) {
			debug.Logf("refresh", "%s expired and needs input, deactivating", sess.Name)
			expired[sess.ID] = true
			continue
		}
		creds, err := s.fetch(ctx, w, sess, "")
		if err != nil {
			debug.Logf("refresh", "%s renewal failed, deactivating: %v", sess.Name, err)
			expired[sess.ID] = true
			continue
		}
		debug.Logf("refresh", "%s renewed until %v", sess.Name, creds.Expiration)
		if err := s.Cache.Put(sess.ID, creds); err != nil {
			return nil, err
		}
		renewed[sess.ID] = creds.Expiration
	}
	if len(renewed) == 0 && len(expired) == 0 {
		return w, nil
	}
	if w, err = s.Load(); err != nil {
		return nil, err
	}
	for i := range w.Sessions {
		sess := &w.Sessions[i]
		if sess.Status != core.StatusActive {
			continue
		}
		if exp, ok := renewed[sess.ID]; ok {
			sess.Expires = exp
		}
		if expired[sess.ID] {
			_ = s.deactivate(sess)
		}
	}
	return w, s.Save(w)
}

// renewable reports whether a session can refresh without user input.
func (s *Service) renewable(sess *core.Session) bool {
	switch sess.Kind {
	case core.KindAWSIAMUser:
		return sess.AWS == nil || sess.AWS.MFADevice == ""
	case core.KindAWSAssumeRole:
		return sess.AWS == nil || sess.AWS.MFADevice == ""
	}
	return true
}

// ReplayOnboarding shows the walkthrough again without touching sessions.
func (s *Service) ReplayOnboarding() error {
	w, err := s.Load()
	if err != nil {
		return err
	}
	w.Onboarded = false
	return s.Save(w)
}

// ResetAll removes every integration and session, their secrets, cached
// credentials, and Rolle-owned AWS profiles, then deletes the workspace file.
func (s *Service) ResetAll() error {
	w, err := s.Load()
	if err != nil {
		return err
	}
	for i := range w.Sessions {
		sess := &w.Sessions[i]
		_ = s.deactivate(sess)
		if sess.Kind == core.KindAWSIAMUser {
			_ = aws.DeleteAccessKey(s.Secrets, sess.ID)
		}
	}
	for _, in := range w.Integrations {
		s.forgetIntegration(in)
	}
	if err := os.RemoveAll(s.Cache.Dir); err != nil {
		return err
	}
	if err := os.Remove(s.WorkspacePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	debug.Logf("reset", "workspace, cache, secrets, and profiles removed")
	s.notify()
	return nil
}

// Settings returns the effective user preferences.
func (s *Service) Settings() (core.Settings, error) {
	w, err := s.Load()
	if err != nil {
		return core.Settings{}, err
	}
	return w.EffectiveSettings(), nil
}

// UpdateSettings stores preferences and applies the ones that take effect immediately.
func (s *Service) UpdateSettings(in core.Settings) (core.Settings, error) {
	w, err := s.Load()
	if err != nil {
		return core.Settings{}, err
	}
	n := in.Normalize()
	if err := netcfg.Apply(n); err != nil {
		return core.Settings{}, err
	}
	w.Settings = &n
	if err := s.Save(w); err != nil {
		return core.Settings{}, err
	}
	debug.Set(n.VerboseLogging)
	debug.Logf("settings", "updated: %+v", n)
	return n, nil
}

// RenameIntegration changes an integration's alias.
func (s *Service) RenameIntegration(ref, alias string) error {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return errors.New("name cannot be empty")
	}
	w, err := s.Load()
	if err != nil {
		return err
	}
	in, err := FindIntegration(w, ref)
	if err != nil {
		return err
	}
	in.Alias = alias
	return s.Save(w)
}

// RenameSession changes a session's name. An active AWS session moves its
// profile to the new name so the shell keeps working.
func (s *Service) RenameSession(ref, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("name cannot be empty")
	}
	w, err := s.Load()
	if err != nil {
		return err
	}
	sess, err := FindSession(w, ref)
	if err != nil {
		return err
	}
	oldProfile := ProfileName(sess)
	sess.Name = name
	if sess.Status == core.StatusActive && sess.Kind.Cloud() == core.CloudAWS && ProfileName(sess) != oldProfile {
		if err := s.writeCloudFiles(sess); err != nil {
			return err
		}
		if err := awsconfig.Remove(s.AWSConfigPath, oldProfile, sess.ID); err != nil {
			return err
		}
	}
	return s.Save(w)
}

// SetFavorite pins or unpins a session.
func (s *Service) SetFavorite(ref string, favorite bool) error {
	w, err := s.Load()
	if err != nil {
		return err
	}
	sess, err := FindSession(w, ref)
	if err != nil {
		return err
	}
	sess.Favorite = favorite
	return s.Save(w)
}

var profileNameRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// SetProfile sets the AWS profile name a session writes. An empty name
// returns to "default". An active session moves its profile section to the
// new name.
func (s *Service) SetProfile(ref, profile string) error {
	profile = strings.TrimSpace(profile)
	if profile != "" && !profileNameRe.MatchString(profile) {
		return errors.New("profile names may contain letters, digits, '.', '-', and '_'")
	}
	w, err := s.Load()
	if err != nil {
		return err
	}
	sess, err := FindSession(w, ref)
	if err != nil {
		return err
	}
	if sess.Kind.Cloud() != core.CloudAWS {
		return fmt.Errorf("%s is not an AWS session", sess.Name)
	}
	oldProfile := ProfileName(sess)
	next := *sess
	next.AWS = &core.AWSSession{}
	*next.AWS = *sess.AWS
	next.AWS.Profile = profile
	newProfile := ProfileName(&next)
	if sess.Status == core.StatusActive && newProfile != oldProfile {
		if err := s.removeCloudFiles(sess); err != nil {
			return err
		}
	}
	sess.AWS.Profile = profile
	if sess.Status == core.StatusActive && newProfile != oldProfile {
		if err := s.writeCloudFiles(sess); err != nil {
			return err
		}
	}
	return s.Save(w)
}

// SetRegion changes an AWS session's region. An active session rewrites its
// profile so the new region applies to the next command.
func (s *Service) SetRegion(ref, region string) error {
	region = strings.TrimSpace(region)
	if region == "" {
		return errors.New("region cannot be empty")
	}
	w, err := s.Load()
	if err != nil {
		return err
	}
	sess, err := FindSession(w, ref)
	if err != nil {
		return err
	}
	if sess.Kind.Cloud() != core.CloudAWS {
		return fmt.Errorf("%s is not an AWS session", sess.Name)
	}
	sess.Region = region
	if sess.Status == core.StatusActive {
		if err := s.writeCloudFiles(sess); err != nil {
			return err
		}
	}
	return s.Save(w)
}

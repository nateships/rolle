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
	"github.com/nateships/rolle/internal/azure"
	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/credcache"
	"github.com/nateships/rolle/internal/debug"
	"github.com/nateships/rolle/internal/gcp"
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

// FindSession resolves a session by exact ID, unique exact name, or unique ID prefix.
func FindSession(w *core.Workspace, ref string) (*core.Session, error) {
	var byName, byPrefix []*core.Session
	for i := range w.Sessions {
		sess := &w.Sessions[i]
		if sess.ID == ref {
			return sess, nil
		}
		if sess.Name == ref {
			byName = append(byName, sess)
		}
		if strings.HasPrefix(sess.ID, ref) {
			byPrefix = append(byPrefix, sess)
		}
	}
	return pick("session", ref, byName, byPrefix)
}

// FindIntegration resolves an integration by exact ID, unique alias, or unique ID prefix.
func FindIntegration(w *core.Workspace, ref string) (*core.Integration, error) {
	var byName, byPrefix []*core.Integration
	for i := range w.Integrations {
		in := &w.Integrations[i]
		if in.ID == ref {
			return in, nil
		}
		if in.Alias == ref {
			byName = append(byName, in)
		}
		if strings.HasPrefix(in.ID, ref) {
			byPrefix = append(byPrefix, in)
		}
	}
	return pick("integration", ref, byName, byPrefix)
}

// pick returns the single name match, else the single ID prefix match. Two
// items with one name are ambiguous: the caller must use the ID.
func pick[T any](kind, ref string, byName, byPrefix []*T) (*T, error) {
	if len(byName) == 1 {
		return byName[0], nil
	}
	if len(byName) > 1 {
		return nil, fmt.Errorf("%s %q: %w (use the ID)", kind, ref, ErrAmbiguous)
	}
	switch len(byPrefix) {
	case 0:
		return nil, fmt.Errorf("%s %q: %w", kind, ref, core.ErrNotFound)
	case 1:
		return byPrefix[0], nil
	}
	return nil, fmt.Errorf("%s %q: %w", kind, ref, ErrAmbiguous)
}

// checkSessionName rejects an empty name or a name another session uses.
func checkSessionName(w *core.Workspace, name, exceptID string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("name cannot be empty")
	}
	for _, sess := range w.Sessions {
		if sess.Name == name && sess.ID != exceptID {
			return fmt.Errorf("a session named %q already exists", name)
		}
	}
	return nil
}

// checkAlias rejects an empty alias or an alias another integration uses.
func checkAlias(w *core.Workspace, alias, exceptID string) error {
	if strings.TrimSpace(alias) == "" {
		return errors.New("name cannot be empty")
	}
	for _, in := range w.Integrations {
		if in.Alias == alias && in.ID != exceptID {
			return fmt.Errorf("an integration named %q already exists", alias)
		}
	}
	return nil
}

var profileNameRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// checkProfile rejects a profile name that is not a valid INI section name.
// Empty means the default profile.
func checkProfile(profile string) error {
	if profile != "" && !profileNameRe.MatchString(profile) {
		return errors.New("profile names may contain letters, digits, '.', '-', and '_'")
	}
	return nil
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
	if err := checkAlias(w, alias, ""); err != nil {
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
	_ = s.forgetIntegration(*in)
	for i := range w.Sessions {
		if w.Sessions[i].IntegrationID == in.ID {
			_ = s.deactivate(&w.Sessions[i])
		}
	}
	// dropDependents edits w.Sessions in place, so collect the IDs first.
	var ids []string
	for _, sess := range w.Sessions {
		if sess.IntegrationID == in.ID {
			ids = append(ids, sess.ID)
		}
	}
	for _, id := range ids {
		s.dropDependents(w, id)
	}
	if err := w.RemoveIntegration(in.ID); err != nil {
		return err
	}
	return s.Save(w)
}

// dropDependents deactivates and removes every assume-role session whose
// source is sourceID, recursively. A chain without its source cannot start.
func (s *Service) dropDependents(w *core.Workspace, sourceID string) {
	for i := 0; i < len(w.Sessions); i++ {
		sess := &w.Sessions[i]
		if sess.AWS == nil || sess.AWS.SourceSessionID != sourceID {
			continue
		}
		debug.Logf("session", "remove %s: its source session is gone", sess.Name)
		_ = s.deactivate(sess)
		id := sess.ID
		w.Sessions = append(w.Sessions[:i], w.Sessions[i+1:]...)
		i--
		s.dropDependents(w, id)
	}
}

func (s *Service) sso(in core.Integration) *aws.SSO {
	return &aws.SSO{Integration: in, Secrets: s.Secrets, Now: s.Now}
}

// SSOLogin starts a browser sign-in for an Identity Center integration. The
// caller completes the returned authorization with Wait; FinishSSOLogin then
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
	type found struct {
		acct aws.Account
		role aws.Role
	}
	var roles []found
	for _, acct := range accounts {
		list, err := sso.ListRoles(ctx, acct.ID)
		if err != nil {
			return nil, err
		}
		for _, role := range list {
			roles = append(roles, found{acct, role})
		}
	}
	// Apply the result to a fresh copy of the workspace so a change made
	// during the network calls is not lost.
	if w, err = s.Load(); err != nil {
		return nil, err
	}
	if in, err = ssoIntegration(w, ref); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var added []core.Session
	for _, f := range roles {
		acct, role := f.acct, f.role
		seen[acct.ID+"/"+role.Name] = true
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
	var gone []string
	kept := w.Sessions[:0]
	for _, sess := range w.Sessions {
		if sess.IntegrationID == in.ID && sess.Kind == core.KindAWSSSORole && (sess.AWS == nil || !seen[sess.AWS.AccountID+"/"+sess.AWS.RoleName]) {
			_ = s.deactivate(&sess)
			gone = append(gone, sess.ID)
			continue
		}
		kept = append(kept, sess)
	}
	w.Sessions = kept
	for _, id := range gone {
		s.dropDependents(w, id)
	}
	if len(added) > 0 || len(gone) > 0 {
		return added, s.Save(w)
	}
	return added, nil
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
	if !strings.HasPrefix(in.RoleARN, "arn:") {
		return core.Session{}, fmt.Errorf("role ARN %q is not an ARN", in.RoleARN)
	}
	if err := checkProfile(in.Profile); err != nil {
		return core.Session{}, err
	}
	w, err := s.Load()
	if err != nil {
		return core.Session{}, err
	}
	if err := checkSessionName(w, in.Name, ""); err != nil {
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
	if err := checkProfile(in.Profile); err != nil {
		return core.Session{}, err
	}
	w, err := s.Load()
	if err != nil {
		return core.Session{}, err
	}
	if err := checkSessionName(w, in.Name, ""); err != nil {
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

// roleSessionName derives the STS RoleSessionName from a session name. STS
// allows 2 to 64 characters from a set that sanitizeProfile already enforces.
func roleSessionName(name string) string {
	out := "rolle-" + sanitizeProfile(name)
	if len(out) > 64 {
		out = out[:64]
	}
	return out
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
	if err := checkSourceProfile(w, sess); err != nil {
		return core.Credentials{}, err
	}
	creds, err := s.fetch(ctx, w, sess, opts.MFACode)
	if err != nil {
		debug.Logf("session", "start %s failed: %v", sess.Name, err)
		if sess.Kind == core.KindAWSSSORole && errors.Is(err, aws.ErrSSOLoginRequired) {
			s.recordSSOToken(sess.IntegrationID)
		}
		return core.Credentials{}, err
	}
	// The fetch can take a while. Apply the result to a fresh copy of the
	// workspace so a change made in the meantime is not lost.
	if w, err = s.Load(); err != nil {
		return core.Credentials{}, err
	}
	if sess, err = w.Session(sess.ID); err != nil {
		return core.Credentials{}, err
	}
	// The fetch may have renewed the portal token silently. Keep the
	// integration's login state in step with the keychain.
	if sess.Kind == core.KindAWSSSORole {
		if in, err := w.Integration(sess.IntegrationID); err == nil {
			in.AWSSSO.TokenExpires = s.sso(*in).TokenExpiry()
		}
	}
	// Write the profile before anything else changes on disk, so a refused
	// profile leaves the other sessions untouched.
	if err := s.writeCloudFiles(sess); err != nil {
		return core.Credentials{}, err
	}
	if err := s.cachePut(sess, creds); err != nil {
		return core.Credentials{}, err
	}
	if err := s.takeOverProfile(w, sess); err != nil {
		return core.Credentials{}, err
	}
	sess.Status = core.StatusActive
	sess.Expires = creds.Expiration
	return creds, s.Save(w)
}

// recordSSOToken writes the portal token state of an integration into the
// workspace, so the interface shows a login that the portal has refused.
func (s *Service) recordSSOToken(integrationID string) {
	w, err := s.Load()
	if err != nil {
		return
	}
	in, err := w.Integration(integrationID)
	if err != nil || in.AWSSSO == nil {
		return
	}
	in.AWSSSO.TokenExpires = s.sso(*in).TokenExpiry()
	_ = s.Save(w)
}

// takeOverProfile stops the other active AWS sessions that write the same
// profile as sess. One active session per profile. Sessions that feed sess
// through a role chain stay active; sess needs them to renew.
func (s *Service) takeOverProfile(w *core.Workspace, sess *core.Session) error {
	if sess.Kind.Cloud() != core.CloudAWS {
		return nil
	}
	sources := sourceChain(w, sess)
	for i := range w.Sessions {
		other := &w.Sessions[i]
		if other.ID == sess.ID || sources[other.ID] || other.Status != core.StatusActive || other.Kind.Cloud() != core.CloudAWS || ProfileName(other) != ProfileName(sess) {
			continue
		}
		debug.Logf("session", "stop %s: profile %s taken over by %s", other.Name, ProfileName(other), sess.Name)
		if err := s.deactivate(other); err != nil {
			return err
		}
	}
	return nil
}

// sourceChain returns the IDs of the sessions that feed sess through a role chain.
func sourceChain(w *core.Workspace, sess *core.Session) map[string]bool {
	sources := map[string]bool{}
	for cur := sess; cur != nil && cur.AWS != nil && cur.AWS.SourceSessionID != ""; {
		src, err := w.Session(cur.AWS.SourceSessionID)
		if err != nil || sources[src.ID] {
			break
		}
		sources[src.ID] = true
		cur = src
	}
	return sources
}

// checkSourceProfile refuses to start a role whose source writes the same AWS
// profile. Both must stay active, and one profile names one session.
func checkSourceProfile(w *core.Workspace, sess *core.Session) error {
	if sess.Kind.Cloud() != core.CloudAWS {
		return nil
	}
	for id := range sourceChain(w, sess) {
		if src, err := w.Session(id); err == nil && ProfileName(src) == ProfileName(sess) {
			return fmt.Errorf("%s and its source %s both write AWS profile %q; set another profile on one of them", sess.Name, src.Name, ProfileName(sess))
		}
	}
	return nil
}

// cacheable reports whether a session's credentials may be written to the
// credential cache. An IAM user without MFA yields its long-lived key, which
// stays in the keychain only.
func cacheable(sess *core.Session) bool {
	return sess.Kind != core.KindAWSIAMUser || (sess.AWS != nil && sess.AWS.MFADevice != "")
}

func (s *Service) cachePut(sess *core.Session, creds core.Credentials) error {
	if !cacheable(sess) {
		return nil
	}
	return s.Cache.Put(sess.ID, creds)
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
	if !cacheable(sess) {
		return creds, nil
	}
	if err := s.Cache.Put(sess.ID, creds); err != nil {
		return core.Credentials{}, err
	}
	sess.Expires = creds.Expiration
	return creds, s.saveExpires(sess.ID, creds.Expiration)
}

// saveExpires records a session's new expiry in a fresh copy of the
// workspace. The caller's copy can predate a network round trip. A write of
// that copy drops changes from the meantime.
func (s *Service) saveExpires(id string, exp *time.Time) error {
	w, err := s.Load()
	if err != nil {
		return err
	}
	sess, err := w.Session(id)
	if err != nil || sess.Status != core.StatusActive {
		return nil
	}
	sess.Expires = exp
	return s.Save(w)
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
			SessionName: roleSessionName(sess.Name),
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
		if sess.Status != core.StatusActive || !s.stale(sess) {
			continue
		}
		if !s.renewable(sess) {
			debug.Logf("refresh", "%s expired and needs input, deactivating", sess.Name)
			expired[sess.ID] = true
			continue
		}
		creds, err := s.fetch(ctx, w, sess, "")
		if err != nil {
			if permanent(err) {
				debug.Logf("refresh", "%s renewal failed, deactivating: %v", sess.Name, err)
				expired[sess.ID] = true
			} else {
				// A network problem is not an expired session. Try again next tick.
				debug.Logf("refresh", "%s renewal failed, will retry: %v", sess.Name, err)
			}
			continue
		}
		debug.Logf("refresh", "%s renewed until %v", sess.Name, creds.Expiration)
		if err := s.cachePut(sess, creds); err != nil {
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
	tokenSeen := map[string]bool{}
	for i := range w.Sessions {
		sess := &w.Sessions[i]
		if sess.Status != core.StatusActive {
			continue
		}
		if exp, ok := renewed[sess.ID]; ok {
			sess.Expires = exp
			// A renewed Identity Center role proves the portal token is valid,
			// possibly through a silent token refresh. Record the new expiry.
			if sess.Kind == core.KindAWSSSORole && !tokenSeen[sess.IntegrationID] {
				tokenSeen[sess.IntegrationID] = true
				if in, err := w.Integration(sess.IntegrationID); err == nil {
					in.AWSSSO.TokenExpires = s.sso(*in).TokenExpiry()
				}
			}
		}
		if expired[sess.ID] {
			_ = s.deactivate(sess)
		}
	}
	return w, s.Save(w)
}

// ReconcileProfiles rewrites the AWS profile of every active AWS session so
// credential_process names this executable. Without this, a profile from a
// binary that moved (a DMG, App Translocation, a Downloads folder) points at
// a path that does not exist.
func (s *Service) ReconcileProfiles() error {
	w, err := s.Load()
	if err != nil {
		return err
	}
	var first error
	for i := range w.Sessions {
		sess := &w.Sessions[i]
		if sess.Status != core.StatusActive || sess.Kind.Cloud() != core.CloudAWS {
			continue
		}
		if err := s.writeCloudFiles(sess); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// stale reports whether a session's credentials need renewal: a cache miss,
// or for an uncached IAM user, an expiry that has passed.
func (s *Service) stale(sess *core.Session) bool {
	if cacheable(sess) {
		_, err := s.Cache.Get(sess.ID)
		return err != nil
	}
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	return sess.Expires == nil || !now.Before(*sess.Expires)
}

// LoginRequired reports whether err means the user has to sign in to the
// provider again before the session can start or renew.
func LoginRequired(err error) bool {
	return errors.Is(err, aws.ErrSSOLoginRequired) || errors.Is(err, azure.ErrLoginRequired) || errors.Is(err, gcp.ErrNoADC)
}

// permanent reports whether a renewal error means the session cannot renew
// without the user: a sign-in is needed, or a source session is gone.
func permanent(err error) bool {
	return errors.Is(err, aws.ErrSSOLoginRequired) || errors.Is(err, azure.ErrLoginRequired) ||
		errors.Is(err, ErrSessionInactive) || errors.Is(err, core.ErrNotFound) ||
		errors.Is(err, aws.ErrNoAccessKey) || errors.Is(err, gcp.ErrNoADC)
}

// renewable reports whether a session can refresh without user input.
func (s *Service) renewable(sess *core.Session) bool {
	switch sess.Kind {
	case core.KindAWSIAMUser, core.KindAWSAssumeRole:
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
	// Remove as much as possible, then report the first failure. The
	// workspace is gone after this, so nothing can retry the cleanup later.
	var first error
	keep := func(err error) {
		if err != nil && first == nil && !errors.Is(err, secrets.ErrNotFound) {
			first = err
		}
	}
	for i := range w.Sessions {
		sess := &w.Sessions[i]
		keep(s.deactivate(sess))
		if sess.Kind == core.KindAWSIAMUser {
			keep(aws.DeleteAccessKey(s.Secrets, sess.ID))
		}
	}
	for _, in := range w.Integrations {
		keep(s.forgetIntegration(in))
	}
	keep(os.RemoveAll(s.Cache.Dir))
	if err := os.Remove(s.WorkspacePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		keep(err)
	}
	debug.Logf("reset", "workspace, cache, secrets, and profiles removed")
	s.notify()
	return first
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
	w, err := s.Load()
	if err != nil {
		return err
	}
	in, err := FindIntegration(w, ref)
	if err != nil {
		return err
	}
	if err := checkAlias(w, alias, in.ID); err != nil {
		return err
	}
	in.Alias = alias
	return s.Save(w)
}

// RenameSession changes a session's name. An active AWS session moves its
// profile to the new name so the shell keeps working.
func (s *Service) RenameSession(ref, name string) error {
	name = strings.TrimSpace(name)
	w, err := s.Load()
	if err != nil {
		return err
	}
	sess, err := FindSession(w, ref)
	if err != nil {
		return err
	}
	if err := checkSessionName(w, name, sess.ID); err != nil {
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

// SetProfile sets the AWS profile name a session writes. An empty name
// returns to "default". An active session moves its profile section to the
// new name and takes it over from any other active session.
func (s *Service) SetProfile(ref, profile string) error {
	profile = strings.TrimSpace(profile)
	if err := checkProfile(profile); err != nil {
		return err
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
	sess.AWS.Profile = profile
	if sess.Status == core.StatusActive && ProfileName(sess) != oldProfile {
		// Write the new section before the old one goes, so a refused name
		// leaves the session on its old profile.
		if err := s.writeCloudFiles(sess); err != nil {
			return err
		}
		if err := awsconfig.Remove(s.AWSConfigPath, oldProfile, sess.ID); err != nil {
			return err
		}
		if err := s.takeOverProfile(w, sess); err != nil {
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

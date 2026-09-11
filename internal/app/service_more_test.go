package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nateships/rolle/internal/aws"
	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/debug"
	"github.com/nateships/rolle/internal/secrets"
)

// addIAMUser creates an IAM user session with a dummy key.
func addIAMUser(t *testing.T, s *Service, name string) core.Session {
	t.Helper()
	sess, err := s.AddIAMUser(AddIAMUserInput{Name: name, Region: "us-east-1", Key: aws.AccessKey{AccessKeyID: "AKIA" + name, SecretAccessKey: "secret"}})
	if err != nil {
		t.Fatal(err)
	}
	return sess
}

// addSession appends a session to the workspace as is.
func addSession(t *testing.T, s *Service, sess core.Session) {
	t.Helper()
	w, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	w.Sessions = append(w.Sessions, sess)
	if err := s.Save(w); err != nil {
		t.Fatal(err)
	}
}

// session reloads one session from the workspace.
func session(t *testing.T, s *Service, ref string) *core.Session {
	t.Helper()
	w, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	sess, err := FindSession(w, ref)
	if err != nil {
		t.Fatal(err)
	}
	return sess
}

func awsConfig(t *testing.T, s *Service) string {
	t.Helper()
	data, err := os.ReadFile(s.AWSConfigPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return string(data)
}

func TestSetProfileValidatesNames(t *testing.T) {
	s := testService(t)
	sess := addIAMUser(t, s, "dev")
	for _, name := range []string{"work", "a.b-c_1", "UPPER", " padded "} {
		if err := s.SetProfile(sess.ID, name); err != nil {
			t.Fatalf("SetProfile(%q) = %v", name, err)
		}
		if got := ProfileName(session(t, s, sess.ID)); got != strings.TrimSpace(name) {
			t.Fatalf("profile after SetProfile(%q) = %q", name, got)
		}
	}
	for _, name := range []string{"bad name", "x/y", "a:b", "tab\tname", "ünïcode"} {
		if err := s.SetProfile(sess.ID, name); err == nil {
			t.Fatalf("SetProfile(%q) accepted an invalid name", name)
		}
	}
	if got := ProfileName(session(t, s, sess.ID)); got != "padded" {
		t.Fatalf("rejected names must not change the profile, got %q", got)
	}
	if err := s.SetProfile(sess.ID, ""); err != nil {
		t.Fatal(err)
	}
	got := session(t, s, sess.ID)
	if got.AWS.Profile != "" || ProfileName(got) != "default" {
		t.Fatalf("empty profile must restore default, got %+v", got.AWS)
	}
	if err := s.SetProfile("nope", "work"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown session: %v", err)
	}
}

func TestSetProfileMovesActiveSection(t *testing.T) {
	s := testService(t)
	sess := addIAMUser(t, s, "dev")
	if _, err := s.Start(context.Background(), sess.ID, StartOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetProfile(sess.ID, "work"); err != nil {
		t.Fatal(err)
	}
	cfg := awsConfig(t, s)
	if !strings.Contains(cfg, "[profile work]") || strings.Contains(cfg, "[default]") || !strings.Contains(cfg, "--session "+sess.ID) {
		t.Fatalf("config after SetProfile(work):\n%s", cfg)
	}
	if err := s.SetProfile(sess.ID, ""); err != nil {
		t.Fatal(err)
	}
	cfg = awsConfig(t, s)
	if !strings.Contains(cfg, "[default]") || strings.Contains(cfg, "[profile work]") {
		t.Fatalf("config after SetProfile(\"\"):\n%s", cfg)
	}
	// The same name again is a no-op for the file.
	if err := s.SetProfile(sess.ID, ""); err != nil {
		t.Fatal(err)
	}
	if session(t, s, sess.ID).Status != core.StatusActive {
		t.Fatal("session must stay active across profile changes")
	}
}

func TestSetProfileAndRegionRejectNonAWSSessions(t *testing.T) {
	s := testService(t)
	addSession(t, s, core.Session{ID: "az1", Name: "sub", Kind: core.KindAzure, Status: core.StatusInactive, Azure: &core.AzureSession{SubscriptionID: "sub", TenantID: "ten"}})
	addSession(t, s, core.Session{ID: "g1", Name: "proj", Kind: core.KindGCP, Status: core.StatusInactive, GCP: &core.GCPSession{ProjectID: "p"}})
	for _, ref := range []string{"az1", "g1"} {
		if err := s.SetProfile(ref, "work"); err == nil || !strings.Contains(err.Error(), "not an AWS session") {
			t.Fatalf("SetProfile(%s) = %v", ref, err)
		}
		if err := s.SetRegion(ref, "eu-west-1"); err == nil || !strings.Contains(err.Error(), "not an AWS session") {
			t.Fatalf("SetRegion(%s) = %v", ref, err)
		}
	}
	if err := s.SetRegion("missing", "eu-west-1"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown session: %v", err)
	}
	if session(t, s, "az1").Region != "" {
		t.Fatal("rejected SetRegion must not change the session")
	}
}

func TestSetFavorite(t *testing.T) {
	s := testService(t)
	sess := addIAMUser(t, s, "dev")
	if session(t, s, sess.ID).Favorite {
		t.Fatal("new sessions are not favorites")
	}
	if err := s.SetFavorite("dev", true); err != nil {
		t.Fatal(err)
	}
	if !session(t, s, sess.ID).Favorite {
		t.Fatal("favorite not set")
	}
	if err := s.SetFavorite(sess.ID[:8], false); err != nil {
		t.Fatal(err)
	}
	if session(t, s, sess.ID).Favorite {
		t.Fatal("favorite not cleared")
	}
	if err := s.SetFavorite("missing", true); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown session: %v", err)
	}
}

func TestRenameIntegrationRejectsEmptyAlias(t *testing.T) {
	s := testService(t)
	in, err := s.AddAWSSSO("acme", "https://acme.awsapps.com/start", "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"", "   ", "\t"} {
		if err := s.RenameIntegration(in.ID, alias); err == nil || !strings.Contains(err.Error(), "cannot be empty") {
			t.Fatalf("RenameIntegration(%q) = %v", alias, err)
		}
	}
	if err := s.RenameIntegration("missing", "x"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown integration: %v", err)
	}
	if err := s.RenameIntegration("acme", "  Acme Corp  "); err != nil {
		t.Fatal(err)
	}
	w, _ := s.Load()
	if got, _ := FindIntegration(w, in.ID); got.Alias != "Acme Corp" {
		t.Fatalf("alias = %q, want trimmed", got.Alias)
	}
}

func TestReplayOnboardingKeepsSessions(t *testing.T) {
	s := testService(t)
	sess := addIAMUser(t, s, "dev")
	w, _ := s.Load()
	w.Onboarded = true
	if err := s.Save(w); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplayOnboarding(); err != nil {
		t.Fatal(err)
	}
	w, _ = s.Load()
	if w.Onboarded {
		t.Fatal("onboarded flag not cleared")
	}
	if len(w.Sessions) != 1 || w.Sessions[0].ID != sess.ID {
		t.Fatalf("sessions changed: %+v", w.Sessions)
	}
}

func TestUpdateSettingsNormalizesBadValues(t *testing.T) {
	// An empty ROLLE_DEBUG lets the stored preference drive the debug switch.
	t.Setenv("ROLLE_DEBUG", "")
	t.Cleanup(func() { debug.Set(false) })
	s := testService(t)
	got, err := s.UpdateSettings(core.Settings{Theme: "neon", AssumeRoleMinutes: 5, Terminal: "iterm", VerboseLogging: true})
	if err != nil {
		t.Fatal(err)
	}
	want := core.Settings{Theme: "system", DefaultRegion: "us-east-1", AssumeRoleMinutes: 60, Terminal: "iterm", VerboseLogging: true}
	if got != want {
		t.Fatalf("normalized = %+v, want %+v", got, want)
	}
	if !debug.Enabled() {
		t.Fatal("verbose logging must enable debug output")
	}
	got, err = s.UpdateSettings(core.Settings{Theme: "dark", DefaultRegion: "eu-west-1", AssumeRoleMinutes: 9999})
	if err != nil {
		t.Fatal(err)
	}
	if got.Theme != "dark" || got.AssumeRoleMinutes != 12*60 || got.DefaultRegion != "eu-west-1" || got.HideOnClose {
		t.Fatalf("clamped = %+v", got)
	}
	if debug.Enabled() {
		t.Fatal("verbose logging off must disable debug output")
	}
	again, err := s.Settings()
	if err != nil || again != got {
		t.Fatalf("reloaded = %+v, %v", again, err)
	}
}

func TestStopRestoresPreexistingDefaultSection(t *testing.T) {
	s := testService(t)
	if err := os.MkdirAll(filepath.Dir(s.AWSConfigPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.AWSConfigPath, []byte("[default]\nregion = ap-south-1\noutput = json\n\n[profile other]\nregion = us-west-2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sess := addIAMUser(t, s, "dev")
	if _, err := s.Start(context.Background(), sess.ID, StartOptions{}); err != nil {
		t.Fatal(err)
	}
	cfg := awsConfig(t, s)
	if !strings.Contains(cfg, "rolle_preexisting") || !strings.Contains(cfg, "region = us-east-1") || !strings.Contains(cfg, "output = json") {
		t.Fatalf("config after start:\n%s", cfg)
	}
	if err := s.Stop(sess.ID); err != nil {
		t.Fatal(err)
	}
	cfg = awsConfig(t, s)
	for _, want := range []string{"[default]", "region = ap-south-1", "output = json", "[profile other]", "region = us-west-2"} {
		if !strings.Contains(cfg, want) {
			t.Fatalf("config after stop lacks %q:\n%s", want, cfg)
		}
	}
	for _, gone := range []string{"credential_process", "rolle_", "us-east-1"} {
		if strings.Contains(cfg, gone) {
			t.Fatalf("config after stop still has %q:\n%s", gone, cfg)
		}
	}
	if err := s.Stop(sess.ID); err != nil {
		t.Fatalf("stopping twice must be harmless: %v", err)
	}
	if err := s.Stop("missing"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown session: %v", err)
	}
}

func TestCredentialsServesCacheAndRejectsInactive(t *testing.T) {
	s := testService(t)
	sess := addIAMUser(t, s, "dev")
	ctx := context.Background()
	if _, err := s.Credentials(ctx, sess.ID); !errors.Is(err, ErrSessionInactive) {
		t.Fatalf("inactive: %v", err)
	}
	if _, err := s.Credentials(ctx, "missing"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown: %v", err)
	}
	if _, err := s.Start(ctx, sess.ID, StartOptions{}); err != nil {
		t.Fatal(err)
	}
	exp := time.Now().Add(time.Hour).UTC()
	if err := s.Cache.Put(sess.ID, core.Credentials{AccessKeyID: "CACHED", SecretAccessKey: "x", Expiration: &exp}); err != nil {
		t.Fatal(err)
	}
	creds, err := s.Credentials(ctx, sess.ID)
	if err != nil || creds.AccessKeyID != "CACHED" {
		t.Fatalf("creds = %+v, %v; want the cached value", creds, err)
	}
	if err := s.Cache.Delete(sess.ID); err != nil {
		t.Fatal(err)
	}
	creds, err = s.Credentials(ctx, sess.ID)
	if err != nil || creds.AccessKeyID != "AKIAdev" {
		t.Fatalf("creds after cache miss = %+v, %v; want a refetch from the stored key", creds, err)
	}
	if _, err := s.Cache.Get(sess.ID); err != nil {
		t.Fatalf("refetch must repopulate the cache: %v", err)
	}
	if err := s.Stop(sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Credentials(ctx, sess.ID); !errors.Is(err, ErrSessionInactive) {
		t.Fatalf("after stop: %v", err)
	}
}

func TestOnChangeFiresOncePerSave(t *testing.T) {
	s := testService(t)
	calls := 0
	s.OnChange = func() { calls++ }
	if _, err := s.Load(); err != nil || calls != 0 {
		t.Fatalf("Load must not notify: %d %v", calls, err)
	}
	w, _ := s.Load()
	if err := s.Save(w); err != nil || calls != 1 {
		t.Fatalf("Save: calls=%d err=%v", calls, err)
	}
	if _, err := s.AddAWSSSO("acme", "https://acme.awsapps.com/start", "us-east-1"); err != nil || calls != 2 {
		t.Fatalf("AddAWSSSO: calls=%d err=%v", calls, err)
	}
	sess := addIAMUser(t, s, "dev")
	if calls != 3 {
		t.Fatalf("AddIAMUser: calls=%d", calls)
	}
	if err := s.SetFavorite(sess.ID, true); err != nil || calls != 4 {
		t.Fatalf("SetFavorite: calls=%d err=%v", calls, err)
	}
	if err := s.SetFavorite("missing", true); err == nil || calls != 4 {
		t.Fatalf("a failed change must not notify: calls=%d err=%v", calls, err)
	}
	if _, err := s.Start(context.Background(), sess.ID, StartOptions{}); err != nil || calls != 5 {
		t.Fatalf("Start: calls=%d err=%v", calls, err)
	}
	if err := s.ResetAll(); err != nil || calls != 6 {
		t.Fatalf("ResetAll: calls=%d err=%v", calls, err)
	}
}

func TestStartSignedOutSSORoleNeedsLogin(t *testing.T) {
	s := testService(t)
	in, err := s.AddAWSSSO("acme", "https://acme.awsapps.com/start", "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	addSession(t, s, core.Session{ID: "r1", Name: "Acme/Admin", Kind: core.KindAWSSSORole, Region: "us-east-1", IntegrationID: in.ID, Status: core.StatusInactive, AWS: &core.AWSSession{AccountID: "1", RoleName: "Admin"}})
	_, err = s.Start(context.Background(), "r1", StartOptions{})
	if !errors.Is(err, aws.ErrSSOLoginRequired) {
		t.Fatalf("Start = %v, want login required", err)
	}
	if got := session(t, s, "r1"); got.Status != core.StatusInactive || got.Expires != nil {
		t.Fatalf("session after failed start = %+v", got)
	}
	if cfg := awsConfig(t, s); strings.Contains(cfg, "credential_process") {
		t.Fatalf("no profile may be written on failure:\n%s", cfg)
	}
	// A token that already expired is the same as no token.
	past := time.Now().Add(-time.Hour)
	if err := s.sso(in).StoreImportedToken("tok", "", "c", "s", "us-east-1", past); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Start(context.Background(), "r1", StartOptions{}); !errors.Is(err, aws.ErrSSOLoginRequired) {
		t.Fatalf("Start with expired token = %v", err)
	}
	// A session that points at a removed integration reports that too.
	addSession(t, s, core.Session{ID: "r2", Name: "orphan", Kind: core.KindAWSSSORole, IntegrationID: "gone", Status: core.StatusInactive, AWS: &core.AWSSession{AccountID: "1", RoleName: "Admin"}})
	if _, err := s.Start(context.Background(), "r2", StartOptions{}); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("Start orphan = %v", err)
	}
}

func TestRemoveIntegrationStopsSessionsAndRemovesProfiles(t *testing.T) {
	s := testService(t)
	in, err := s.AddAWSSSO("acme", "https://acme.awsapps.com/start", "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.sso(in).StoreImportedToken("tok", "", "c", "s", "us-east-1", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	exp := time.Now().Add(time.Hour).UTC()
	role := core.Session{ID: "r1", Name: "Acme/Admin", Kind: core.KindAWSSSORole, Region: "us-east-1", IntegrationID: in.ID, Status: core.StatusActive, Expires: &exp, AWS: &core.AWSSession{AccountID: "1", RoleName: "Admin", Profile: "acme-admin"}}
	addSession(t, s, role)
	if err := s.Cache.Put(role.ID, core.Credentials{AccessKeyID: "A", Expiration: &exp}); err != nil {
		t.Fatal(err)
	}
	if err := s.writeCloudFiles(&role); err != nil {
		t.Fatal(err)
	}
	other := addIAMUser(t, s, "keep")
	if _, err := s.Start(context.Background(), other.ID, StartOptions{}); err != nil {
		t.Fatal(err)
	}
	if cfg := awsConfig(t, s); !strings.Contains(cfg, "[profile acme-admin]") {
		t.Fatalf("setup config:\n%s", cfg)
	}
	if err := s.RemoveIntegration(in.ID[:8]); err != nil {
		t.Fatal(err)
	}
	w, _ := s.Load()
	if len(w.Integrations) != 0 || len(w.Sessions) != 1 || w.Sessions[0].ID != other.ID {
		t.Fatalf("workspace after remove = %+v", w)
	}
	if _, err := s.Cache.Get(role.ID); err == nil {
		t.Fatal("cached credentials of the removed session must be dropped")
	}
	if _, err := s.Secrets.Get("aws-sso-token/" + in.ID); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("token must be removed: %v", err)
	}
	cfg := awsConfig(t, s)
	if strings.Contains(cfg, "acme-admin") || !strings.Contains(cfg, "--session "+other.ID) {
		t.Fatalf("config after remove:\n%s", cfg)
	}
	if err := s.RemoveIntegration("missing"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown integration: %v", err)
	}
}

func TestFindIntegrationByAliasIDAndPrefix(t *testing.T) {
	w := &core.Workspace{Integrations: []core.Integration{{ID: "abc123", Alias: "acme"}, {ID: "abx456", Alias: "beta"}, {ID: "zzz", Alias: "abx"}}}
	cases := map[string]string{"abc123": "acme", "acme": "acme", "abc": "acme", "abx4": "beta", "abx": "abx"}
	for ref, alias := range cases {
		got, err := FindIntegration(w, ref)
		if err != nil || got.Alias != alias {
			t.Fatalf("FindIntegration(%q) = %v, %v; want %s", ref, got, err, alias)
		}
	}
	if _, err := FindIntegration(w, "ab"); !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("ambiguous prefix: %v", err)
	}
	if _, err := FindIntegration(w, "nope"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown: %v", err)
	}
	if _, err := FindSession(w, "x"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("FindSession on an empty workspace: %v", err)
	}
}

func TestAddAssumeRoleValidation(t *testing.T) {
	s := testService(t)
	if _, err := s.AddAssumeRole(AddAssumeRoleInput{Name: "x", RoleARN: "arn:aws:iam::1:role/x"}); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("missing source: %v", err)
	}
	if _, err := s.AddAssumeRole(AddAssumeRoleInput{Name: "x", RoleARN: "arn:aws:iam::1:role/x", SourceRef: "ghost"}); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown source: %v", err)
	}
	addSession(t, s, core.Session{ID: "az1", Name: "sub", Kind: core.KindAzure, Status: core.StatusInactive, Azure: &core.AzureSession{SubscriptionID: "sub"}})
	if _, err := s.AddAssumeRole(AddAssumeRoleInput{Name: "x", RoleARN: "arn:aws:iam::1:role/x", SourceRef: "az1"}); err == nil || !strings.Contains(err.Error(), "not an AWS session") {
		t.Fatalf("azure source: %v", err)
	}
	w, _ := s.Load()
	if len(w.Sessions) != 1 {
		t.Fatalf("rejected input must not add sessions: %+v", w.Sessions)
	}
	src := addIAMUser(t, s, "src")
	sess, err := s.AddAssumeRole(AddAssumeRoleInput{Name: "admin", Region: "eu-west-1", RoleARN: "arn:aws:iam::1:role/admin", SourceRef: src.ID[:8], ExternalID: "ext", Profile: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	if sess.Kind != core.KindAWSAssumeRole || sess.AWS.SourceSessionID != src.ID || sess.AWS.ExternalID != "ext" || sess.AWS.Profile != "admin" || sess.Status != core.StatusInactive {
		t.Fatalf("session = %+v", sess)
	}
	// Chaining from another role is allowed.
	if _, err := s.AddAssumeRole(AddAssumeRoleInput{Name: "deeper", Region: "eu-west-1", RoleARN: "arn:aws:iam::2:role/x", SourceRef: "admin"}); err != nil {
		t.Fatal(err)
	}
}

func TestAddGCPImpersonationValidation(t *testing.T) {
	s := testService(t)
	if _, err := s.AddGCPImpersonation(AddGCPImpersonationInput{Name: "x", IntegrationRef: "gcp", ProjectID: "p", ServiceAccount: "sa@p"}); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown integration: %v", err)
	}
	if _, err := s.AddAWSSSO("acme", "https://acme.awsapps.com/start", "us-east-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddGCPImpersonation(AddGCPImpersonationInput{Name: "x", IntegrationRef: "acme", ProjectID: "p", ServiceAccount: "sa@p"}); err == nil || !strings.Contains(err.Error(), "not a Google Cloud account") {
		t.Fatalf("aws integration: %v", err)
	}
	w, _ := s.Load()
	if len(w.Sessions) != 0 {
		t.Fatalf("rejected input must not add sessions: %+v", w.Sessions)
	}
}

func TestRemoveSessionUnknownAndAssumeRoleCleanup(t *testing.T) {
	s := testService(t)
	if err := s.RemoveSession("missing"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown session: %v", err)
	}
	src := addIAMUser(t, s, "src")
	role, err := s.AddAssumeRole(AddAssumeRoleInput{Name: "admin", Region: "us-east-1", RoleARN: "arn:aws:iam::1:role/admin", SourceRef: src.ID, Profile: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	// Mark the role active by hand so RemoveSession has files to clean up.
	exp := time.Now().Add(time.Hour).UTC()
	w, _ := s.Load()
	got, _ := FindSession(w, role.ID)
	got.Status, got.Expires = core.StatusActive, &exp
	if err := s.Save(w); err != nil {
		t.Fatal(err)
	}
	if err := s.Cache.Put(role.ID, core.Credentials{AccessKeyID: "A", Expiration: &exp}); err != nil {
		t.Fatal(err)
	}
	if err := s.writeCloudFiles(got); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveSession("admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Cache.Get(role.ID); err == nil {
		t.Fatal("cache entry must be removed")
	}
	if cfg := awsConfig(t, s); strings.Contains(cfg, "[profile admin]") {
		t.Fatalf("profile must be removed:\n%s", cfg)
	}
	if _, err := aws.LoadAccessKey(s.Secrets, src.ID); err != nil {
		t.Fatalf("the source key must survive: %v", err)
	}
}

func TestRefreshLeavesInactiveAndFreshSessionsAlone(t *testing.T) {
	s := testService(t)
	calls := 0
	s.OnChange = func() { calls++ }
	inactive := addIAMUser(t, s, "idle")
	active := addIAMUser(t, s, "busy")
	if _, err := s.Start(context.Background(), active.ID, StartOptions{}); err != nil {
		t.Fatal(err)
	}
	before := calls
	w, err := s.Refresh()
	if err != nil {
		t.Fatal(err)
	}
	if calls != before {
		t.Fatal("Refresh must not save when nothing changed")
	}
	a, _ := FindSession(w, active.ID)
	i, _ := FindSession(w, inactive.ID)
	if a.Status != core.StatusActive || i.Status != core.StatusInactive {
		t.Fatalf("statuses after refresh: %s %s", a.Status, i.Status)
	}
}

func TestLoadRejectsCorruptWorkspace(t *testing.T) {
	s := testService(t)
	if err := os.MkdirAll(filepath.Dir(s.WorkspacePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.WorkspacePath, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(); err == nil {
		t.Fatal("corrupt workspace accepted")
	}
	if _, err := s.Settings(); err == nil {
		t.Fatal("Settings must surface the load error")
	}
	if err := s.SetFavorite("x", true); err == nil {
		t.Fatal("SetFavorite must surface the load error")
	}
}

func TestAddAssumeRoleRejectsEmptyNameAndBadARN(t *testing.T) {
	s := testService(t)
	src, _ := s.AddIAMUser(AddIAMUserInput{Name: "src", Region: "us-east-1", Key: aws.AccessKey{AccessKeyID: "A", SecretAccessKey: "B"}})
	if _, err := s.AddAssumeRole(AddAssumeRoleInput{Name: " ", RoleARN: "arn:aws:iam::1:role/x", SourceRef: src.ID}); err == nil {
		t.Fatal("empty name accepted")
	}
	if _, err := s.AddAssumeRole(AddAssumeRoleInput{Name: "x", RoleARN: "role/x", SourceRef: src.ID}); err == nil {
		t.Fatal("bad ARN accepted")
	}
}

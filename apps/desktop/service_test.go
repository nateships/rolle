package main

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nateships/rolle/internal/app"
	"github.com/nateships/rolle/internal/aws"
	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/credcache"
	"github.com/nateships/rolle/internal/secrets"
	"github.com/nateships/rolle/internal/version"
)

// testrolle returns a RolleService on a throwaway workspace, AWS config, and
// in-memory secret store. It has no Wails app, so window and event calls stay
// guarded. HOME and the cloud CLI config paths point at empty directories so
// no test reads the real machine.
func testrolle(t *testing.T) *RolleService {
	t.Helper()
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(home, ".aws", "config"))
	t.Setenv("AZURE_CONFIG_DIR", filepath.Join(home, ".azure"))
	t.Setenv("LEAPP_HOME", filepath.Join(home, ".leapp"))
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", filepath.Join(home, "no-adc.json"))
	svc := &app.Service{
		WorkspacePath: filepath.Join(dir, "workspace.json"),
		AWSConfigPath: filepath.Join(dir, "aws", "config"),
		Executable:    "/opt/rolle",
		Secrets:       &secrets.Memory{},
		Cache:         &credcache.Cache{Store: &secrets.Memory{}, Dir: filepath.Join(dir, "cache")},
	}
	return NewRolleService(svc)
}

func addIAMUser(t *testing.T, r *RolleService, name string) core.Session {
	t.Helper()
	sess, err := r.AddIAMUser(IAMUserInput{Name: name, Region: "us-east-1", AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	return sess
}

func TestServiceIdentity(t *testing.T) {
	r := testrolle(t)
	if r.ServiceName() != "rolle" {
		t.Fatal(r.ServiceName())
	}
	if r.DevMode() != devMode {
		t.Fatal("DevMode disagrees with the build tag")
	}
	info := r.Info()
	if info.Version != version.Version || info.WorkspacePath != r.svc.WorkspacePath || info.CacheDir != r.svc.Cache.Dir || info.AWSConfigPath != r.svc.AWSConfigPath {
		t.Fatalf("info = %+v", info)
	}
	if !strings.Contains(r.SupportURL(), "template=bug.yml") {
		t.Fatal(r.SupportURL())
	}
}

func TestDemoModeFollowsEnvironment(t *testing.T) {
	r := testrolle(t)
	t.Setenv("ROLLE_DEMO", "")
	if r.DemoMode() {
		t.Fatal("demo mode on without ROLLE_DEMO")
	}
	t.Setenv("ROLLE_DEMO", "1")
	if !r.DemoMode() {
		t.Fatal("demo mode off with ROLLE_DEMO=1")
	}
	// Demo builds never read the real machine or run gcloud.
	if got := r.Discover(); len(got.AWSPortals) != 0 || got.GCP != nil {
		t.Fatalf("demo discover = %+v", got)
	}
	if res, err := r.ImportLeappSessions(); err != nil || len(res.Sessions) != 0 {
		t.Fatalf("demo leapp import = %+v %v", res, err)
	}
	if err := r.GCloudLogin(); err != nil {
		t.Fatal(err)
	}
}

func TestOnboardingFlagRoundTrips(t *testing.T) {
	r := testrolle(t)
	w, err := r.Workspace()
	if err != nil || w.Onboarded {
		t.Fatalf("fresh workspace = %+v %v", w, err)
	}
	if err := r.CompleteOnboarding(); err != nil {
		t.Fatal(err)
	}
	if w, _ = r.Workspace(); !w.Onboarded {
		t.Fatal("onboarding not saved")
	}
	if err := r.ReplayOnboarding(); err != nil {
		t.Fatal(err)
	}
	if w, _ = r.Workspace(); w.Onboarded {
		t.Fatal("replay did not clear the flag")
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	r := testrolle(t)
	st, err := r.Settings()
	if err != nil || st.Theme != "system" || st.AssumeRoleMinutes != 60 {
		t.Fatalf("defaults = %+v %v", st, err)
	}
	st.Theme = "light"
	st.UpdateChannel = "beta"
	st.AutoUpdateOff = true
	got, err := r.UpdateSettings(st)
	if err != nil {
		t.Fatal(err)
	}
	if got.Theme != "light" || got.UpdateChannel != "beta" || !got.AutoUpdateOff {
		t.Fatalf("updated = %+v", got)
	}
	if again, _ := r.Settings(); !reflect.DeepEqual(again, got) {
		t.Fatalf("stored = %+v, want %+v", again, got)
	}
}

func TestAWSSSOIntegrationLifecycle(t *testing.T) {
	r := testrolle(t)
	in, err := r.AddAWSSSO("acme", "https://acme.awsapps.com/start", "us-east-1")
	if err != nil || in.AWSSSO == nil {
		t.Fatalf("add = %+v %v", in, err)
	}
	if err := r.RenameIntegration(in.ID, "acme-prod"); err != nil {
		t.Fatal(err)
	}
	w, _ := r.Workspace()
	if len(w.Integrations) != 1 || w.Integrations[0].Alias != "acme-prod" {
		t.Fatalf("integrations = %+v", w.Integrations)
	}
	// Login needs the network, but an unknown portal fails before that and
	// leaves nothing pending.
	if _, err := r.StartSSOLogin("nope"); err == nil {
		t.Fatal("login on unknown portal accepted")
	}
	if len(r.pending) != 0 {
		t.Fatal("failed login left a pending entry")
	}
	if _, err := r.WaitSSOLogin(in.ID); err == nil || !strings.Contains(err.Error(), "no login in progress") {
		t.Fatalf("wait without start: %v", err)
	}
	// Cancel drops a pending login and tolerates a missing one.
	r.pending[in.ID] = &pendingLogin{auth: &aws.DeviceAuthorization{}}
	r.CancelSSOLogin(in.ID)
	r.CancelSSOLogin(in.ID)
	if len(r.pending) != 0 {
		t.Fatal("cancel kept the pending login")
	}
	if _, err := r.SyncSSO("nope"); err == nil {
		t.Fatal("sync on unknown portal accepted")
	}
	if err := r.SSOLogout("nope"); err == nil {
		t.Fatal("logout on unknown portal accepted")
	}
	if err := r.RemoveIntegration(in.ID); err != nil {
		t.Fatal(err)
	}
	if w, _ = r.Workspace(); len(w.Integrations) != 0 {
		t.Fatalf("integrations after remove = %+v", w.Integrations)
	}
}

func TestImportAWSSSOWithoutCLITokenRegistersOnly(t *testing.T) {
	r := testrolle(t)
	res, err := r.ImportAWSSSO("acme", "https://acme.awsapps.com/start", "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	if res.LoggedIn || res.Integration.Alias != "acme" || len(res.Sessions) != 0 {
		t.Fatalf("import = %+v", res)
	}
}

func TestIAMUserSessionLifecycle(t *testing.T) {
	r := testrolle(t)
	sess := addIAMUser(t, r, "dev user")
	if sess.Kind != core.KindAWSIAMUser || sess.Region != "us-east-1" {
		t.Fatalf("session = %+v", sess)
	}
	if _, err := r.EnvText(sess.ID); err == nil {
		t.Fatal("env for an inactive session succeeded")
	}
	if err := r.OpenTerminal(sess.ID); err == nil {
		t.Fatal("terminal for an inactive session succeeded")
	}
	creds, err := r.Start(sess.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if creds.AccessKeyID != "AKIAEXAMPLE" {
		t.Fatalf("creds = %+v", creds)
	}
	env, err := r.EnvText(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(env, "AWS_ACCESS_KEY_ID") || !strings.Contains(env, "AKIAEXAMPLE") || !strings.Contains(env, "AWS_REGION") {
		t.Fatalf("env:\n%s", env)
	}
	w, _ := r.Workspace()
	if w.Sessions[0].Status != core.StatusActive {
		t.Fatalf("status = %s", w.Sessions[0].Status)
	}
	if err := r.Stop(sess.ID); err != nil {
		t.Fatal(err)
	}
	w, _ = r.Workspace()
	if w.Sessions[0].Status != core.StatusInactive {
		t.Fatalf("status after stop = %s", w.Sessions[0].Status)
	}
	if err := r.RemoveSession(sess.ID); err != nil {
		t.Fatal(err)
	}
	if w, _ = r.Workspace(); len(w.Sessions) != 0 {
		t.Fatalf("sessions after remove = %+v", w.Sessions)
	}
}

func TestAssumeRoleChainsFromSource(t *testing.T) {
	r := testrolle(t)
	src := addIAMUser(t, r, "src")
	chained, err := r.AddAssumeRole(app.AddAssumeRoleInput{Name: "admin", Region: "us-east-1", RoleARN: "arn:aws:iam::123456789012:role/Admin", SourceRef: src.ID})
	if err != nil {
		t.Fatal(err)
	}
	if chained.Kind != core.KindAWSAssumeRole || chained.AWS.SourceSessionID != src.ID {
		t.Fatalf("chained = %+v", chained)
	}
	if err := r.RemoveSession(src.ID); err == nil {
		t.Fatal("removing a source session succeeded")
	}
}

func TestSessionSetters(t *testing.T) {
	r := testrolle(t)
	sess := addIAMUser(t, r, "personal")
	if err := r.SetFavorite(sess.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := r.SetRegion(sess.ID, "eu-west-1"); err != nil {
		t.Fatal(err)
	}
	if err := r.SetProfile(sess.ID, "work"); err != nil {
		t.Fatal(err)
	}
	if err := r.RenameSession(sess.ID, "work user"); err != nil {
		t.Fatal(err)
	}
	w, _ := r.Workspace()
	got := w.Sessions[0]
	if !got.Favorite || got.Region != "eu-west-1" || got.AWS.Profile != "work" || got.Name != "work user" {
		t.Fatalf("session = %+v aws = %+v", got, got.AWS)
	}
	if err := r.SetProfile(sess.ID, ""); err != nil {
		t.Fatal(err)
	}
	if w, _ = r.Workspace(); w.Sessions[0].AWS.Profile != "" {
		t.Fatalf("profile not cleared: %q", w.Sessions[0].AWS.Profile)
	}
	if err := r.SetFavorite("nope", true); err == nil {
		t.Fatal("favorite on unknown session accepted")
	}
}

func TestResetRemovesEverything(t *testing.T) {
	r := testrolle(t)
	addIAMUser(t, r, "gone")
	if _, err := r.AddAWSSSO("acme", "https://acme.awsapps.com/start", "us-east-1"); err != nil {
		t.Fatal(err)
	}
	if err := r.Reset(); err != nil {
		t.Fatal(err)
	}
	w, err := r.Workspace()
	if err != nil || len(w.Sessions) != 0 || len(w.Integrations) != 0 {
		t.Fatalf("after reset = %+v %v", w, err)
	}
}

func TestAzureCallsRejectUnknownTenant(t *testing.T) {
	r := testrolle(t)
	in, err := r.AddAzure("contoso", "7a1c2e40-3d5b-4f6a-9b8c-0d1e2f3a4b5c")
	if err != nil || in.Azure == nil || in.Azure.TenantID != "7a1c2e40-3d5b-4f6a-9b8c-0d1e2f3a4b5c" {
		t.Fatalf("add = %+v %v", in, err)
	}
	if _, err := r.AzureLogin("nope"); err == nil {
		t.Fatal("login on unknown tenant accepted")
	}
	if err := r.AzureLogout("nope"); err == nil {
		t.Fatal("logout on unknown tenant accepted")
	}
	if _, err := r.SyncAzure("nope"); err == nil {
		t.Fatal("sync on unknown tenant accepted")
	}
}

func TestGCPWithoutCredentials(t *testing.T) {
	r := testrolle(t)
	st := r.GCPStatus()
	if st.Ready || st.Account != "" || st.LoginCommand == "" || st.InstallURL == "" {
		t.Fatalf("status = %+v", st)
	}
	if _, err := r.AddGCP("gcp"); err == nil {
		t.Fatal("AddGCP without ADC succeeded")
	}
	if _, err := r.SyncGCP("nope"); err == nil {
		t.Fatal("sync on unknown integration accepted")
	}
	if _, err := r.AddGCPImpersonation(app.AddGCPImpersonationInput{IntegrationRef: "nope", Name: "deployer"}); err == nil {
		t.Fatal("impersonation on unknown integration accepted")
	}
}

func TestDiscoverOnEmptyMachine(t *testing.T) {
	r := testrolle(t)
	t.Setenv("ROLLE_DEMO", "")
	got := r.Discover()
	if len(got.AWSPortals) != 0 || len(got.AzureTenants) != 0 || got.GCP != nil {
		t.Fatalf("discover = %+v", got)
	}
	// No Leapp workspace means nothing to import, not an error.
	res, err := r.ImportLeappSessions()
	if err != nil || len(res.Sessions) != 0 || len(res.Skipped) != 0 {
		t.Fatalf("leapp import = %+v %v", res, err)
	}
}

func TestOpenCallsRefuseBadInput(t *testing.T) {
	r := testrolle(t)
	if err := r.OpenURL("file:///etc/passwd"); err == nil {
		t.Fatal("non-http URL opened")
	}
	if err := r.OpenConsole("nope"); err == nil {
		t.Fatal("console for unknown session opened")
	}
}

func TestAWSLoginSessionNeedsBrowserFirst(t *testing.T) {
	r := testrolle(t)
	sess, err := r.AddAWSLogin(AWSLoginInput{Name: "console", Region: "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}
	if sess.Kind != core.KindAWSLogin || sess.Region != "us-east-1" {
		t.Fatalf("session = %+v", sess)
	}
	// Nothing is stored, so the start reports that a sign-in is needed.
	if _, err := r.Start(sess.ID, ""); !app.LoginRequired(err) {
		t.Fatalf("Start = %v", err)
	}
	if _, err := r.WaitSessionLogin(sess.ID); err == nil || !strings.Contains(err.Error(), "no login in progress") {
		t.Fatalf("WaitSessionLogin without a start = %v", err)
	}
	if _, err := r.StartSessionLogin("nope"); err == nil {
		t.Fatal("StartSessionLogin on an unknown session succeeded")
	}
	if err := r.RemoveSession(sess.ID); err != nil {
		t.Fatal(err)
	}
}

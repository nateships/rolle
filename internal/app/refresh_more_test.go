package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nateships/rolle/internal/aws"
	"github.com/nateships/rolle/internal/azure"
	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/discover"
	"github.com/nateships/rolle/internal/gcp"
)

func TestLoginRequiredAndPermanentClassifyErrors(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		login     bool
		permanent bool
	}{
		{"sso login", fmt.Errorf("start: %w", aws.ErrSSOLoginRequired), true, true},
		{"azure login", fmt.Errorf("%w: refused", azure.ErrLoginRequired), true, true},
		{"gcp adc", gcp.ErrNoADC, true, true},
		{"source inactive", fmt.Errorf("src: %w", ErrSessionInactive), false, true},
		{"integration gone", core.ErrNotFound, false, true},
		{"key gone", aws.ErrNoAccessKey, false, true},
		{"network", errors.New("dial tcp: i/o timeout"), false, false},
		{"nil", nil, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := LoginRequired(tc.err); got != tc.login {
				t.Fatalf("LoginRequired = %v, want %v", got, tc.login)
			}
			if got := permanent(tc.err); got != tc.permanent {
				t.Fatalf("permanent = %v, want %v", got, tc.permanent)
			}
		})
	}
}

func TestRenewableNeedsNoInput(t *testing.T) {
	s := testService(t)
	mfa := &core.AWSSession{MFADevice: "arn:aws:iam::1:mfa/me"}
	cases := []struct {
		name string
		sess core.Session
		want bool
	}{
		{"iam user", core.Session{Kind: core.KindAWSIAMUser, AWS: &core.AWSSession{}}, true},
		{"iam user with mfa", core.Session{Kind: core.KindAWSIAMUser, AWS: mfa}, false},
		{"role", core.Session{Kind: core.KindAWSAssumeRole, AWS: &core.AWSSession{}}, true},
		{"role with mfa", core.Session{Kind: core.KindAWSAssumeRole, AWS: mfa}, false},
		{"sso role", core.Session{Kind: core.KindAWSSSORole, AWS: mfa}, true},
		{"azure", core.Session{Kind: core.KindAzure}, true},
		{"gcp", core.Session{Kind: core.KindGCP}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := s.renewable(&tc.sess); got != tc.want {
				t.Fatalf("renewable = %v, want %v", got, tc.want)
			}
		})
	}
}

// ssoRole adds an active Identity Center role for in with no cached
// credentials, so the next Refresh must renew it.
func ssoRole(t *testing.T, s *Service, in core.Integration) core.Session {
	t.Helper()
	past := time.Now().Add(-time.Minute).UTC()
	sess := core.Session{ID: "r1", Name: "Acme/Admin", Kind: core.KindAWSSSORole, Region: "us-east-1", IntegrationID: in.ID, Status: core.StatusActive, Expires: &past, AWS: &core.AWSSession{AccountID: "111111111111", RoleName: "Admin"}}
	addSession(t, s, sess)
	return sess
}

func TestRefreshRenewsSSORoleAndRecordsPortalToken(t *testing.T) {
	fakeHome(t)
	startFakeSSO(t, "Admin")
	s := testService(t)
	in, err := s.AddAWSSSO("acme", portalURL, "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	tokenExp := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	if err := s.sso(in).StoreImportedToken(cliToken, "", "", "", "", tokenExp); err != nil {
		t.Fatal(err)
	}
	sess := ssoRole(t, s, in)
	w, err := s.Refresh()
	if err != nil {
		t.Fatal(err)
	}
	got, _ := FindSession(w, sess.ID)
	if got.Status != core.StatusActive || got.Expires == nil || !got.Expires.After(time.Now().Add(30*time.Minute)) {
		t.Fatalf("session after refresh = %+v", got)
	}
	creds, err := s.Cache.Get(sess.ID)
	if err != nil || creds.AccessKeyID != "ASIAFAKE" {
		t.Fatalf("cache after refresh = %+v, %v", creds, err)
	}
	// A successful renewal proves the portal token, so the integration shows
	// the login even though the token was stored behind its back.
	integ, _ := FindIntegration(w, "acme")
	if integ.AWSSSO.TokenExpires == nil || !integ.AWSSSO.TokenExpires.Equal(tokenExp) {
		t.Fatalf("TokenExpires = %v, want %v", integ.AWSSSO.TokenExpires, tokenExp)
	}
	// A second Refresh finds fresh credentials and changes nothing.
	calls := 0
	s.OnChange = func() { calls++ }
	if _, err := s.Refresh(); err != nil || calls != 0 {
		t.Fatalf("second refresh: err=%v saves=%d", err, calls)
	}
}

func TestRefreshKeepsSessionWhenPortalIsUnreachable(t *testing.T) {
	fakeHome(t)
	closed := httptest.NewServer(nil)
	closed.Close()
	t.Setenv("AWS_ENDPOINT_URL_SSO", closed.URL)
	t.Setenv("AWS_MAX_ATTEMPTS", "1")
	s := testService(t)
	in, err := s.AddAWSSSO("acme", portalURL, "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.sso(in).StoreImportedToken(cliToken, "", "", "", "", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	sess := ssoRole(t, s, in)
	calls := 0
	s.OnChange = func() { calls++ }
	w, err := s.Refresh()
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("a network failure must not rewrite the workspace")
	}
	if got, _ := FindSession(w, sess.ID); got.Status != core.StatusActive {
		t.Fatalf("a network failure deactivated the session: %+v", got)
	}
	if _, err := s.Cache.Get(sess.ID); err == nil {
		t.Fatal("nothing was fetched, so nothing may be cached")
	}
}

func TestRefreshDeactivatesSSORoleThatNeedsLogin(t *testing.T) {
	fakeHome(t)
	startFakeSSO(t, "Admin")
	s := testService(t)
	in, err := s.AddAWSSSO("acme", portalURL, "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	sess := ssoRole(t, s, in)
	if err := s.writeCloudFiles(&sess); err != nil {
		t.Fatal(err)
	}
	w, err := s.Refresh()
	if err != nil {
		t.Fatal(err)
	}
	got, _ := FindSession(w, sess.ID)
	if got.Status != core.StatusInactive || got.Expires != nil {
		t.Fatalf("session without a portal token = %+v", got)
	}
	// The list points at a session that ended by itself, until a stop by hand.
	if got.ExpiredAt == nil {
		t.Fatalf("expired session carries no ExpiredAt: %+v", got)
	}
	if cfg := awsConfig(t, s); strings.Contains(cfg, "credential_process") {
		t.Fatalf("profile of a deactivated session remains:\n%s", cfg)
	}
	if err := s.Stop(sess.ID); err != nil {
		t.Fatal(err)
	}
	w, _ = s.Load()
	if got, _ := FindSession(w, sess.ID); got.ExpiredAt != nil {
		t.Fatalf("manual stop kept ExpiredAt: %+v", got)
	}
}

func TestRefreshDeactivatesRoleWhoseSourceStopped(t *testing.T) {
	s := testService(t)
	src := addIAMUser(t, s, "src")
	role, err := s.AddAssumeRole(AddAssumeRoleInput{Name: "admin", Region: "us-east-1", RoleARN: "arn:aws:iam::1:role/admin", SourceRef: src.ID})
	if err != nil {
		t.Fatal(err)
	}
	w, _ := s.Load()
	r, _ := w.Session(role.ID)
	r.Status = core.StatusActive
	if err := s.Save(w); err != nil {
		t.Fatal(err)
	}
	w, err = s.Refresh()
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := FindSession(w, role.ID); got.Status != core.StatusInactive {
		t.Fatalf("role with a stopped source = %+v", got)
	}
}

func TestStartFailureRecordsRefusedPortalToken(t *testing.T) {
	s := testService(t)
	in, err := s.AddAWSSSO("acme", portalURL, "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	// The interface believes the portal is logged in.
	w, _ := s.Load()
	future := time.Now().Add(time.Hour).UTC()
	w.Integrations[0].AWSSSO.TokenExpires = &future
	if err := s.Save(w); err != nil {
		t.Fatal(err)
	}
	// The keychain holds a token that already expired and cannot refresh.
	if err := s.sso(in).StoreImportedToken("stale", "", "", "", "", time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	addSession(t, s, core.Session{ID: "r1", Name: "Acme/Admin", Kind: core.KindAWSSSORole, IntegrationID: in.ID, Status: core.StatusInactive, AWS: &core.AWSSession{AccountID: "1", RoleName: "Admin"}})
	if _, err := s.Start(context.Background(), "r1", StartOptions{}); !errors.Is(err, aws.ErrSSOLoginRequired) {
		t.Fatalf("Start = %v", err)
	}
	w, _ = s.Load()
	if integ, _ := FindIntegration(w, "acme"); integ.AWSSSO.TokenExpires != nil {
		t.Fatalf("TokenExpires = %v, want nil after the portal refused the token", integ.AWSSSO.TokenExpires)
	}

	// Unknown or non-portal integrations are ignored without a write.
	az, err := s.AddAzure("contoso", "")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	s.OnChange = func() { calls++ }
	s.clearSSOLogin("missing")
	s.clearSSOLogin(az.ID)
	if calls != 0 {
		t.Fatalf("clearSSOLogin saved %d times for integrations without a portal", calls)
	}
}

// A refused refresh token stays in the keychain and still looks renewable.
// The workspace must not read it back as a login that renews itself.
func TestStartFailureClearsRefusedRefreshToken(t *testing.T) {
	fakeHome(t)
	portal := startFakeSSO(t, "Admin")
	portal.tokenStatus = http.StatusBadRequest
	s := testService(t)
	in, err := s.AddAWSSSO("acme", portalURL, "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.sso(in).StoreImportedToken("old", "rt", "cid", "csecret", "us-east-1", time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	addSession(t, s, core.Session{ID: "r1", Name: "Acme/Admin", Kind: core.KindAWSSSORole, IntegrationID: in.ID, Status: core.StatusInactive, AWS: &core.AWSSession{AccountID: "1", RoleName: "Admin"}})
	if _, err := s.Start(context.Background(), "r1", StartOptions{}); !errors.Is(err, aws.ErrSSOLoginRequired) {
		t.Fatalf("Start = %v", err)
	}
	w, _ := s.Load()
	if integ, _ := FindIntegration(w, "acme"); integ.AWSSSO.TokenExpires != nil || integ.AWSSSO.Renews {
		t.Fatalf("login = %v renews=%v, want cleared after the refused refresh", integ.AWSSSO.TokenExpires, integ.AWSSSO.Renews)
	}
}

func TestStartTakesOverSharedProfile(t *testing.T) {
	s := testService(t)
	a := addIAMUser(t, s, "a")
	b := addIAMUser(t, s, "b")
	ctx := context.Background()
	if _, err := s.Start(ctx, a.ID, StartOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Start(ctx, b.ID, StartOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := session(t, s, a.ID); got.Status != core.StatusInactive || got.Expires != nil {
		t.Fatalf("a must lose the default profile: %+v", got)
	}
	if got := session(t, s, b.ID); got.Status != core.StatusActive {
		t.Fatalf("b = %+v", got)
	}
	cfg := awsConfig(t, s)
	if strings.Count(cfg, "credential_process") != 1 || !strings.Contains(cfg, "--session "+b.ID) {
		t.Fatalf("config after take-over:\n%s", cfg)
	}

	// A session on another profile leaves b alone, and moving it onto the
	// default profile takes that over as well.
	c, err := s.AddIAMUser(AddIAMUserInput{Name: "c", Region: "us-east-1", Profile: "work", Key: aws.AccessKey{AccessKeyID: "AKIAc", SecretAccessKey: "s"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Start(ctx, c.ID, StartOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := session(t, s, b.ID); got.Status != core.StatusActive {
		t.Fatal("a different profile must not stop b")
	}
	if err := s.SetProfile(c.ID, ""); err != nil {
		t.Fatal(err)
	}
	if got := session(t, s, b.ID); got.Status != core.StatusInactive {
		t.Fatal("moving c onto the default profile must stop b")
	}
	cfg = awsConfig(t, s)
	if strings.Contains(cfg, "[profile work]") || strings.Count(cfg, "credential_process") != 1 || !strings.Contains(cfg, "--session "+c.ID) {
		t.Fatalf("config after SetProfile:\n%s", cfg)
	}
}

func TestReconcileProfilesRewritesExecutable(t *testing.T) {
	s := testService(t)
	active := addIAMUser(t, s, "active")
	idle := addIAMUser(t, s, "idle")
	if _, err := s.Start(context.Background(), active.ID, StartOptions{}); err != nil {
		t.Fatal(err)
	}
	s.Executable = "/Applications/rolle.app/Contents/MacOS/rolle"
	if err := s.ReconcileProfiles(); err != nil {
		t.Fatal(err)
	}
	cfg := awsConfig(t, s)
	if !strings.Contains(cfg, s.Executable+" creds --session "+active.ID) || strings.Contains(cfg, "/opt/rolle") {
		t.Fatalf("config after reconcile:\n%s", cfg)
	}
	if strings.Contains(cfg, idle.ID) {
		t.Fatal("an inactive session got a profile")
	}
	// A refused write is reported, the other profiles are still tried.
	s.AWSConfigPath = filepath.Join(t.TempDir(), "blocker")
	if err := os.MkdirAll(s.AWSConfigPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := s.ReconcileProfiles(); err == nil {
		t.Fatal("expected an error when the config path is a directory")
	}
}

func TestOpenTerminalNeedsAnActiveSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH lookups differ on Windows")
	}
	t.Setenv("PATH", t.TempDir())
	t.Setenv("TERMINAL", filepath.Join(t.TempDir(), "absent"))
	t.Setenv("TERM_PROGRAM", "Apple_Terminal")
	s := testService(t)
	if _, err := s.UpdateSettings(core.Settings{Terminal: "terminal"}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.OpenTerminal(ctx, "ghost"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown = %v", err)
	}
	dev := addIAMUser(t, s, "dev")
	if err := s.OpenTerminal(ctx, dev.ID); !errors.Is(err, ErrSessionInactive) {
		t.Fatalf("inactive = %v", err)
	}
	if _, err := s.Start(ctx, dev.ID, StartOptions{}); err != nil {
		t.Fatal(err)
	}
	// No terminal can start in this environment. The launcher still
	// prepares its private directory and leaves no script behind.
	if err := s.OpenTerminal(ctx, dev.ID); err == nil {
		t.Fatal("expected an error without a terminal on PATH")
	}
	launch := filepath.Join(s.Cache.Dir, "launch")
	info, err := os.Stat(launch)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("launch dir: %v %v", info, err)
	}
	if entries, _ := os.ReadDir(launch); len(entries) != 0 {
		t.Fatalf("scripts left behind: %v", entries)
	}
}

func TestBrokenWorkspaceIsReportedByEveryOperation(t *testing.T) {
	s := testService(t)
	if err := os.MkdirAll(filepath.Dir(s.WorkspacePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.WorkspacePath, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	calls := map[string]func() error{
		"Load":           func() error { _, err := s.Load(); return err },
		"Settings":       func() error { _, err := s.Settings(); return err },
		"UpdateSettings": func() error { _, err := s.UpdateSettings(core.Settings{}); return err },
		"AddAWSSSO":      func() error { _, err := s.AddAWSSSO("a", "https://x.awsapps.com/start", "us-east-1"); return err },
		"AddAzure":       func() error { _, err := s.AddAzure("a", ""); return err },
		"AddIAMUser":     func() error { _, err := s.AddIAMUser(AddIAMUserInput{Name: "a", Region: "us-east-1"}); return err },
		"AddAssumeRole": func() error {
			_, err := s.AddAssumeRole(AddAssumeRoleInput{Name: "a", RoleARN: "arn:x", SourceRef: "s"})
			return err
		},
		"AddGCPImpersonation": func() error { _, err := s.AddGCPImpersonation(AddGCPImpersonationInput{}); return err },
		"RemoveIntegration":   func() error { return s.RemoveIntegration("a") },
		"RemoveSession":       func() error { return s.RemoveSession("a") },
		"RenameIntegration":   func() error { return s.RenameIntegration("a", "b") },
		"RenameSession":       func() error { return s.RenameSession("a", "b") },
		"SetFavorite":         func() error { return s.SetFavorite("a", true) },
		"SetProfile":          func() error { return s.SetProfile("a", "p") },
		"SetRegion":           func() error { return s.SetRegion("a", "us-east-1") },
		"Start":               func() error { _, err := s.Start(ctx, "a", StartOptions{}); return err },
		"Stop":                func() error { return s.Stop("a") },
		"Credentials":         func() error { _, err := s.Credentials(ctx, "a"); return err },
		"SessionEnv":          func() error { _, err := s.SessionEnv(ctx, "a"); return err },
		"ConsoleURL":          func() error { _, err := s.ConsoleURL(ctx, "a"); return err },
		"ConsoleURLFor":       func() error { _, err := s.ConsoleURLFor(ctx, "a"); return err },
		"OpenTerminal":        func() error { return s.OpenTerminal(ctx, "a") },
		"Refresh":             func() error { _, err := s.Refresh(); return err },
		"ReconcileProfiles":   func() error { return s.ReconcileProfiles() },
		"ReplayOnboarding":    func() error { return s.ReplayOnboarding() },
		"ResetAll":            func() error { return s.ResetAll() },
		"SSOLogin":            func() error { _, err := s.SSOLogin(ctx, "a"); return err },
		"SSODeviceLogin":      func() error { _, err := s.SSODeviceLogin(ctx, "a"); return err },
		"FinishSSOLogin":      func() error { _, err := s.FinishSSOLogin(ctx, "a"); return err },
		"SSOLogout":           func() error { return s.SSOLogout("a") },
		"SyncSSO":             func() error { _, err := s.SyncSSO(ctx, "a"); return err },
		"AzureLogin":          func() error { _, err := s.AzureLogin(ctx, "a"); return err },
		"AzureDeviceLogin":    func() error { _, err := s.AzureDeviceLogin(ctx, "a"); return err },
		"FinishAzureLogin":    func() error { _, err := s.FinishAzureLogin(ctx, "a", "me"); return err },
		"AzureLogout":         func() error { return s.AzureLogout(ctx, "a") },
		"SyncAzure":           func() error { _, err := s.SyncAzure(ctx, "a"); return err },
		"SyncGCP":             func() error { _, err := s.SyncGCP(ctx, "a"); return err },
		"ImportLeappSessions": func() error { _, err := s.ImportLeappSessions(&discover.LeappWorkspace{}); return err },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			err := call()
			if err == nil || !strings.Contains(err.Error(), "parse "+s.WorkspacePath) {
				t.Fatalf("%s = %v, want the parse error", name, err)
			}
		})
	}
	// The broken file is still there. Nothing replaced it with an empty workspace.
	if data, _ := os.ReadFile(s.WorkspacePath); string(data) != "{not json" {
		t.Fatalf("workspace file changed to %q", data)
	}
}

func TestFetchCloudErrorsPerKind(t *testing.T) {
	fakeHome(t)
	s := testService(t)
	az, err := s.AddAzure("contoso", "t-1")
	if err != nil {
		t.Fatal(err)
	}
	addSession(t, s, core.Session{ID: "az", Name: "sub", Kind: core.KindAzure, IntegrationID: az.ID, Azure: &core.AzureSession{SubscriptionID: "s", TenantID: "t-1"}})
	addSession(t, s, core.Session{ID: "gcp", Name: "proj", Kind: core.KindGCP, GCP: &core.GCPSession{ProjectID: "p"}})
	addSession(t, s, core.Session{ID: "odd", Name: "odd", Kind: core.Kind("weird")})
	addSession(t, s, core.Session{ID: "orphan", Name: "orphan", Kind: core.KindAzure, IntegrationID: "gone", Azure: &core.AzureSession{}})
	ctx := context.Background()
	_, err = s.Start(ctx, "az", StartOptions{})
	if !errors.Is(err, azure.ErrLoginRequired) || !LoginRequired(err) {
		t.Fatalf("azure without login = %v", err)
	}
	_, err = s.Start(ctx, "gcp", StartOptions{})
	if !errors.Is(err, gcp.ErrNoADC) || !LoginRequired(err) {
		t.Fatalf("gcp without ADC = %v", err)
	}
	if _, err := s.Start(ctx, "odd", StartOptions{}); err == nil || !strings.Contains(err.Error(), "not supported yet") {
		t.Fatalf("unknown kind = %v", err)
	}
	if _, err := s.Start(ctx, "orphan", StartOptions{}); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("orphan azure session = %v", err)
	}
	for _, id := range []string{"az", "gcp", "odd"} {
		if got := session(t, s, id); got.Status == core.StatusActive {
			t.Fatalf("%s became active after a failed start", id)
		}
	}
}

func TestCloudLookupsGuardIntegrationKind(t *testing.T) {
	fakeHome(t)
	s := testService(t)
	sso, err := s.AddAWSSSO("acme", portalURL, "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	az, err := s.AddAzure("contoso", "")
	if err != nil {
		t.Fatal(err)
	}
	w, _ := s.Load()
	w.Integrations = append(w.Integrations, core.Integration{ID: "g1", Alias: "gcp", Cloud: core.CloudGCP, GCP: &core.GCPIntegration{}})
	if err := s.Save(w); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	kind := func(name string, err error, want string) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s = %v, want %q", name, err, want)
		}
	}
	_, err = s.SSOLogin(ctx, az.ID)
	kind("SSOLogin on azure", err, "not an AWS IAM Identity Center portal")
	_, err = s.SSODeviceLogin(ctx, "gcp")
	kind("SSODeviceLogin on gcp", err, "not an AWS IAM Identity Center portal")
	_, err = s.SyncAzure(ctx, sso.ID)
	kind("SyncAzure on sso", err, "not an Azure tenant")
	_, err = s.AzureDeviceLogin(ctx, "gcp")
	kind("AzureDeviceLogin on gcp", err, "not an Azure tenant")
	_, err = s.SyncGCP(ctx, az.ID)
	kind("SyncGCP on azure", err, "not a Google Cloud account")
	if _, err := s.AzureLogin(ctx, "ghost"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("AzureLogin unknown = %v", err)
	}
	if _, err := s.SyncAzure(ctx, az.ID); !errors.Is(err, azure.ErrLoginRequired) {
		t.Fatalf("SyncAzure without login = %v", err)
	}
	if _, err := s.SyncGCP(ctx, "gcp"); !errors.Is(err, gcp.ErrNoADC) {
		t.Fatalf("SyncGCP without ADC = %v", err)
	}
	if _, _, err := s.AddGCP(ctx, "gcp2"); !errors.Is(err, gcp.ErrNoADC) {
		t.Fatalf("AddGCP without ADC = %v", err)
	}
	w, _ = s.Load()
	if len(w.Integrations) != 3 {
		t.Fatalf("AddGCP without ADC saved an integration: %+v", w.Integrations)
	}
	// FinishAzureLogin records the account before discovery, so a failed
	// discovery still leaves the sign-in visible.
	if _, err := s.FinishAzureLogin(ctx, az.ID, "me@contoso.com"); !errors.Is(err, azure.ErrLoginRequired) {
		t.Fatalf("FinishAzureLogin = %v", err)
	}
	w, _ = s.Load()
	if got, _ := FindIntegration(w, az.ID); got.Azure.Account != "me@contoso.com" {
		t.Fatalf("account = %q", got.Azure.Account)
	}
}

func TestAddGCPRecordsAccountBeforeDiscovery(t *testing.T) {
	home := fakeHome(t)
	adc := filepath.Join(home, "adc.json")
	writeFile(t, adc, `{"type":"authorized_user","client_id":"c","client_secret":"s","refresh_token":"r","account":"me@example.com"}`)
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", adc)
	s := testService(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the token exchange fails before any network call
	in, added, err := s.AddGCP(ctx, "gcp")
	if err == nil || errors.Is(err, gcp.ErrNoADC) || len(added) != 0 {
		t.Fatalf("AddGCP offline = %v, %v", added, err)
	}
	if in.Cloud != core.CloudGCP || in.GCP.Account != "me@example.com" {
		t.Fatalf("integration = %+v", in)
	}
	w, _ := s.Load()
	if got, err := FindIntegration(w, "gcp"); err != nil || got.GCP.Account != "me@example.com" {
		t.Fatalf("saved integration = %+v, %v", got, err)
	}
	if _, _, err := s.AddGCP(ctx, "gcp"); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("duplicate alias = %v", err)
	}
}

func TestConsoleURLRejectsSessionsWithoutARole(t *testing.T) {
	s := testService(t)
	user := addIAMUser(t, s, "user")
	addSession(t, s, core.Session{ID: "az", Name: "sub", Kind: core.KindAzure, Azure: &core.AzureSession{TenantID: "t"}})
	addSession(t, s, core.Session{ID: "odd", Name: "odd", Kind: core.Kind("weird")})
	ctx := context.Background()
	if _, err := s.ConsoleURL(ctx, user.ID); err == nil || !strings.Contains(err.Error(), "needs a role") {
		t.Fatalf("iam user = %v", err)
	}
	if _, err := s.ConsoleURL(ctx, "az"); err == nil || !strings.Contains(err.Error(), "only available for AWS") {
		t.Fatalf("azure through ConsoleURL = %v", err)
	}
	if _, err := s.ConsoleURLFor(ctx, "odd"); err == nil || !strings.Contains(err.Error(), "no console for odd") {
		t.Fatalf("unknown kind = %v", err)
	}
	if _, err := s.ConsoleURLFor(ctx, "ghost"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown session = %v", err)
	}
}

func TestRenameSessionValidation(t *testing.T) {
	s := testService(t)
	a := addIAMUser(t, s, "a")
	addIAMUser(t, s, "b")
	if err := s.RenameSession(a.ID, "b"); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("duplicate = %v", err)
	}
	if err := s.RenameSession(a.ID, "  "); err == nil {
		t.Fatal("blank name accepted")
	}
	if err := s.RenameSession("ghost", "c"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown = %v", err)
	}
	if err := s.RenameSession(a.ID, " a2 "); err != nil {
		t.Fatal(err)
	}
	if got := session(t, s, a.ID); got.Name != "a2" {
		t.Fatalf("name = %q", got.Name)
	}
}

func TestDefaultUsesEnvironmentPaths(t *testing.T) {
	home := fakeHome(t)
	ws := filepath.Join(home, "ws", "workspace.json")
	cache := filepath.Join(home, "cache")
	cfg := filepath.Join(home, "aws-config")
	t.Setenv("ROLLE_WORKSPACE", ws)
	t.Setenv("ROLLE_CACHE_DIR", cache)
	t.Setenv("AWS_CONFIG_FILE", cfg)
	s, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if s.WorkspacePath != ws || s.Cache.Dir != cache || s.AWSConfigPath != cfg {
		t.Fatalf("paths = %s %s %s", s.WorkspacePath, s.Cache.Dir, s.AWSConfigPath)
	}
	if s.Executable == "" || s.Secrets == nil {
		t.Fatalf("service = %+v", s)
	}
	// A missing workspace is an empty one; nothing is created on disk yet.
	if _, err := os.Stat(ws); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("Default must not write the workspace")
	}
}

func TestCredentialsRefreshRecordsNewExpiry(t *testing.T) {
	fakeHome(t)
	startFakeSSO(t, "Admin")
	s := testService(t)
	in, err := s.AddAWSSSO("acme", portalURL, "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.sso(in).StoreImportedToken(cliToken, "", "", "", "", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	sess := ssoRole(t, s, in)
	// credential_process asks for an active session whose cache is empty.
	creds, err := s.Credentials(context.Background(), sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if creds.AccessKeyID != "ASIAFAKE" || creds.Expiration == nil {
		t.Fatalf("creds = %+v", creds)
	}
	if cached, err := s.Cache.Get(sess.ID); err != nil || cached.AccessKeyID != "ASIAFAKE" {
		t.Fatalf("cache = %+v, %v", cached, err)
	}
	if got := session(t, s, sess.ID); got.Expires == nil || !got.Expires.Equal(*creds.Expiration) {
		t.Fatalf("expiry not recorded: %+v", got)
	}
	// A session stopped in the meantime keeps its inactive state.
	if err := s.Stop(sess.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.saveExpires(sess.ID, creds.Expiration); err != nil {
		t.Fatal(err)
	}
	if got := session(t, s, sess.ID); got.Status != core.StatusInactive || got.Expires != nil {
		t.Fatalf("saveExpires revived a stopped session: %+v", got)
	}
}

func TestRefreshProbesIdleSSOLogins(t *testing.T) {
	cases := []struct {
		name        string
		tokenStatus int
		wantLogin   bool
		// secondCalls is the /token count after a second Refresh: a refused
		// login is not probed again, a renewed one is valid, a fault retries.
		secondCalls int
	}{
		{"refused refresh clears the login", http.StatusBadRequest, false, 1},
		{"service fault keeps the login", http.StatusInternalServerError, true, 2},
		{"silent refresh renews the login", http.StatusOK, true, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fakeHome(t)
			portal := startFakeSSO(t)
			portal.tokenStatus = tc.tokenStatus
			s := testService(t)
			in, err := s.AddAWSSSO("acme", portalURL, "us-east-1")
			if err != nil {
				t.Fatal(err)
			}
			// The access token lapsed an hour ago, but the refresh token is
			// there, so the workspace still records a login.
			past := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
			if err := s.sso(in).StoreImportedToken("old", "rt-1", "cid", "csecret", "us-east-1", past); err != nil {
				t.Fatal(err)
			}
			w, err := s.Load()
			if err != nil {
				t.Fatal(err)
			}
			s.recordLogin(&w.Integrations[0])
			if w.Integrations[0].AWSSSO.TokenExpires == nil || !w.Integrations[0].AWSSSO.Renews {
				t.Fatalf("setup: login must be recorded as renewing: %+v", w.Integrations[0].AWSSSO)
			}
			if err := s.Save(w); err != nil {
				t.Fatal(err)
			}

			w, err = s.Refresh()
			if err != nil {
				t.Fatal(err)
			}
			integ, _ := FindIntegration(w, "acme")
			if got := integ.AWSSSO.TokenExpires != nil; got != tc.wantLogin {
				t.Fatalf("TokenExpires = %v, want login %v", integ.AWSSSO.TokenExpires, tc.wantLogin)
			}
			if tc.tokenStatus == http.StatusOK && !integ.AWSSSO.TokenExpires.After(time.Now().Add(50*time.Minute)) {
				t.Fatalf("renewed TokenExpires = %v", integ.AWSSSO.TokenExpires)
			}
			if portal.tokenCalls != 1 {
				t.Fatalf("token calls = %d", portal.tokenCalls)
			}
			if _, err := s.Refresh(); err != nil {
				t.Fatal(err)
			}
			if portal.tokenCalls != tc.secondCalls {
				t.Fatalf("token calls after second refresh = %d, want %d", portal.tokenCalls, tc.secondCalls)
			}
		})
	}
}

func TestRefreshDecaysOldExpiredAt(t *testing.T) {
	s := testService(t)
	now := time.Now()
	s.Now = func() time.Time { return now }
	old := now.Add(-expiredTTL - time.Minute)
	fresh := now.Add(-expiredTTL + time.Minute)
	w, _ := s.Load()
	w.Sessions = []core.Session{
		{ID: "old", Name: "old", Kind: core.KindAWSIAMUser, Status: core.StatusInactive, ExpiredAt: &old, AWS: &core.AWSSession{}},
		{ID: "fresh", Name: "fresh", Kind: core.KindAWSIAMUser, Status: core.StatusInactive, ExpiredAt: &fresh, AWS: &core.AWSSession{}},
	}
	if err := s.Save(w); err != nil {
		t.Fatal(err)
	}
	calls := 0
	s.OnChange = func() { calls++ }
	if _, err := s.Refresh(); err != nil {
		t.Fatal(err)
	}
	// The decay is a workspace change, so it must be saved.
	if calls != 1 {
		t.Fatalf("OnChange calls = %d, want 1", calls)
	}
	w, _ = s.Load()
	if got, _ := FindSession(w, "old"); got.ExpiredAt != nil {
		t.Fatalf("ExpiredAt older than expiredTTL kept: %+v", got)
	}
	if got, _ := FindSession(w, "fresh"); got.ExpiredAt == nil {
		t.Fatalf("ExpiredAt within expiredTTL dropped: %+v", got)
	}
	// Nothing left to decay, so a second refresh saves nothing.
	if _, err := s.Refresh(); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("OnChange calls after idle refresh = %d, want 1", calls)
	}
}

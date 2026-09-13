package app

import (
	"context"
	"encoding/json"
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
	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/secrets"
)

const (
	portalURL = "https://acme.awsapps.com/start"
	cliToken  = "cli-access-token"
)

// fakeHome points every home-relative lookup at an empty directory so tests
// never read the real ~/.aws, ~/.azure, gcloud, or Leapp files. It returns
// the directory.
func fakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	t.Setenv("AWS_CONFIG_FILE", "")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(home, ".aws", "credentials"))
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AZURE_CONFIG_DIR", "")
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("LEAPP_HOME", filepath.Join(home, ".Leapp"))
	return home
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

// writeCLITokenCache puts an AWS CLI SSO token for portalURL in the fake home.
func writeCLITokenCache(t *testing.T, home string, expires time.Time) {
	t.Helper()
	tok := map[string]string{
		"startUrl": portalURL + "/", "region": "us-east-1", "accessToken": cliToken,
		"expiresAt": expires.UTC().Format(time.RFC3339), "clientId": "cid", "clientSecret": "csecret", "refreshToken": "rt",
	}
	data, _ := json.Marshal(tok)
	writeFile(t, filepath.Join(home, ".aws", "sso", "cache", "abc.json"), string(data))
}

// fakeSSO is a loopback stand-in for the IAM Identity Center portal API. It
// serves one account with the roles in roles and issues fixed credentials.
type fakeSSO struct {
	roles []string
	calls int
	// tokenStatus is the OIDC /token reply: 0 or 200 issues a fresh access
	// token, 400 refuses the refresh token, 500 fails the request with an
	// error the SDK does not retry, so a fault costs one call.
	tokenStatus int
	tokenCalls  int
}

func (f *fakeSSO) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.calls++
	if r.URL.Path == "/token" {
		f.tokenCalls++
		w.Header().Set("Content-Type", "application/json")
		switch f.tokenStatus {
		case http.StatusBadRequest:
			w.Header().Set("x-amzn-ErrorType", "InvalidGrantException")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"invalid_grant","error_description":"refresh token expired"}`)
		case http.StatusInternalServerError:
			w.Header().Set("x-amzn-ErrorType", "InvalidRequestException")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"invalid_request","error_description":"service fault stand-in"}`)
		default:
			fmt.Fprintf(w, `{"accessToken":%q,"tokenType":"Bearer","expiresIn":3600,"refreshToken":"rt-2"}`, cliToken)
		}
		return
	}
	if r.Header.Get("x-amz-sso_bearer_token") != cliToken {
		w.Header().Set("x-amzn-ErrorType", "UnauthorizedException")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"message":"Session token not found or invalid"}`)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/assignment/accounts":
		fmt.Fprint(w, `{"accountList":[{"accountId":"111111111111","accountName":"Acme","emailAddress":"ops@acme.example"}]}`)
	case "/assignment/roles":
		var roles []string
		for _, name := range f.roles {
			roles = append(roles, fmt.Sprintf(`{"accountId":%q,"roleName":%q}`, r.URL.Query().Get("account_id"), name))
		}
		fmt.Fprintf(w, `{"roleList":[%s]}`, strings.Join(roles, ","))
	case "/federation/credentials":
		exp := time.Now().Add(time.Hour).UnixMilli()
		fmt.Fprintf(w, `{"roleCredentials":{"accessKeyId":"ASIAFAKE","secretAccessKey":"fake-secret","sessionToken":"fake-token","expiration":%d}}`, exp)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// startFakeSSO serves the portal API on loopback and routes the AWS SDK to it.
func startFakeSSO(t *testing.T, roles ...string) *fakeSSO {
	t.Helper()
	f := &fakeSSO{roles: roles}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	t.Setenv("AWS_ENDPOINT_URL_SSO", srv.URL)
	t.Setenv("AWS_ENDPOINT_URL_SSO_OIDC", srv.URL)
	return f
}

func TestImportAWSSSOReusesCLIToken(t *testing.T) {
	home := fakeHome(t)
	writeCLITokenCache(t, home, time.Now().Add(2*time.Hour))
	startFakeSSO(t, "Admin", "ReadOnly")
	s := testService(t)
	res, err := s.ImportAWSSSO(context.Background(), "acme", portalURL, "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	if !res.LoggedIn || res.Integration.Alias != "acme" || res.Integration.AWSSSO.StartURL != portalURL {
		t.Fatalf("result = %+v", res)
	}
	if len(res.Sessions) != 2 || res.Sessions[0].Name != "Acme/Admin" || res.Sessions[1].Name != "Acme/ReadOnly" {
		t.Fatalf("sessions = %+v", res.Sessions)
	}
	raw, err := s.Secrets.Get("aws-sso-token/" + res.Integration.ID)
	if err != nil {
		t.Fatalf("token not stored in the memory store: %v", err)
	}
	var stored struct {
		AccessToken, ClientID, ClientSecret, RefreshToken, Region string
		Expires                                                   time.Time
	}
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.AccessToken != cliToken || stored.ClientID != "cid" || stored.ClientSecret != "csecret" || stored.RefreshToken != "rt" || stored.Region != "us-east-1" || !stored.Expires.After(time.Now()) {
		t.Fatalf("stored token = %+v", stored)
	}
	w, _ := s.Load()
	in, _ := FindIntegration(w, "acme")
	if in.AWSSSO.TokenExpires == nil || !in.AWSSSO.TokenExpires.Equal(stored.Expires) {
		t.Fatalf("TokenExpires = %v, want %v", in.AWSSSO.TokenExpires, stored.Expires)
	}
	for _, sess := range w.Sessions {
		if sess.Kind != core.KindAWSSSORole || sess.IntegrationID != in.ID || sess.Status != core.StatusInactive || sess.Region != "us-east-1" || sess.AWS.AccountID != "111111111111" {
			t.Fatalf("session = %+v", sess)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".aws", "config")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("import must not write the AWS CLI config in the fake home")
	}
}

func TestImportAWSSSOWithoutCLITokenNeedsLogin(t *testing.T) {
	home := fakeHome(t)
	// An expired CLI token does not count.
	writeCLITokenCache(t, home, time.Now().Add(-time.Minute))
	f := startFakeSSO(t, "Admin")
	s := testService(t)
	res, err := s.ImportAWSSSO(context.Background(), "acme", portalURL, "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	if res.LoggedIn || len(res.Sessions) != 0 {
		t.Fatalf("result = %+v", res)
	}
	if f.calls != 0 {
		t.Fatalf("the portal was called %d times without a token", f.calls)
	}
	if _, err := s.Secrets.Get("aws-sso-token/" + res.Integration.ID); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("no token may be stored: %v", err)
	}
	w, _ := s.Load()
	if len(w.Integrations) != 1 || w.Integrations[0].AWSSSO.TokenExpires != nil {
		t.Fatalf("integrations = %+v", w.Integrations)
	}
}

func TestImportAWSSSODropsRejectedCLIToken(t *testing.T) {
	home := fakeHome(t)
	writeCLITokenCache(t, home, time.Now().Add(2*time.Hour))
	f := startFakeSSO(t, "Admin")
	// The cache holds a valid-looking token the portal no longer accepts.
	writeFile(t, filepath.Join(home, ".aws", "sso", "cache", "abc.json"), strings.ReplaceAll(readFile(t, filepath.Join(home, ".aws", "sso", "cache", "abc.json")), cliToken, "revoked"))
	s := testService(t)
	res, err := s.ImportAWSSSO(context.Background(), "acme", portalURL, "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	if res.LoggedIn || len(res.Sessions) != 0 || f.calls == 0 {
		t.Fatalf("result = %+v, calls = %d", res, f.calls)
	}
	if _, err := s.Secrets.Get("aws-sso-token/" + res.Integration.ID); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("rejected token must be dropped: %v", err)
	}
	w, _ := s.Load()
	if w.Integrations[0].AWSSSO.TokenExpires != nil || len(w.Sessions) != 0 {
		t.Fatalf("workspace = %+v", w)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestSyncSSOAddsAndRemovesRoles(t *testing.T) {
	home := fakeHome(t)
	writeCLITokenCache(t, home, time.Now().Add(2*time.Hour))
	f := startFakeSSO(t, "Admin", "ReadOnly")
	s := testService(t)
	res, err := s.ImportAWSSSO(context.Background(), "acme", portalURL, "us-east-1")
	if err != nil || len(res.Sessions) != 2 {
		t.Fatalf("import = %+v, %v", res, err)
	}
	// Give the Admin role local state that a sync must keep.
	if err := s.SetFavorite("Acme/Admin", true); err != nil {
		t.Fatal(err)
	}
	if err := s.RenameSession("Acme/Admin", "prod admin"); err != nil {
		t.Fatal(err)
	}
	f.roles = []string{"Admin", "Billing"}
	added, err := s.SyncSSO(context.Background(), "acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 1 || added[0].Name != "Acme/Billing" {
		t.Fatalf("added = %+v", added)
	}
	w, _ := s.Load()
	var names []string
	for _, sess := range w.Sessions {
		names = append(names, sess.Name)
	}
	if strings.Join(names, ",") != "prod admin,Acme/Billing" {
		t.Fatalf("sessions after sync = %v", names)
	}
	if !w.Sessions[0].Favorite {
		t.Fatal("existing session must keep its favorite flag")
	}
	if _, err := s.SyncSSO(context.Background(), "missing"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown integration: %v", err)
	}
}

func TestStartSSORoleWritesProfileAndStopClears(t *testing.T) {
	home := fakeHome(t)
	writeCLITokenCache(t, home, time.Now().Add(2*time.Hour))
	startFakeSSO(t, "Admin")
	s := testService(t)
	if _, err := s.ImportAWSSSO(context.Background(), "acme", portalURL, "us-east-1"); err != nil {
		t.Fatal(err)
	}
	creds, err := s.Start(context.Background(), "Acme/Admin", StartOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if creds.AccessKeyID != "ASIAFAKE" || creds.SessionToken != "fake-token" || creds.Expiration == nil {
		t.Fatalf("creds = %+v", creds)
	}
	got := session(t, s, "Acme/Admin")
	if got.Status != core.StatusActive || got.Expires == nil {
		t.Fatalf("session = %+v", got)
	}
	cfg := awsConfig(t, s)
	if !strings.Contains(cfg, "[default]") || !strings.Contains(cfg, "--session "+got.ID) || !strings.Contains(cfg, "region = us-east-1") {
		t.Fatalf("config:\n%s", cfg)
	}
	env, err := s.SessionEnv(context.Background(), got.ID)
	if err != nil || env[0][1] != "ASIAFAKE" || env[2][1] != "fake-token" {
		t.Fatalf("env = %v, %v", env, err)
	}
	if err := s.SSOLogout("acme"); err != nil {
		t.Fatal(err)
	}
	if got := session(t, s, "Acme/Admin"); got.Status != core.StatusInactive {
		t.Fatalf("logout must stop the session: %+v", got)
	}
	if cfg := awsConfig(t, s); strings.Contains(cfg, "credential_process") {
		t.Fatalf("profile must be removed on logout:\n%s", cfg)
	}
	w, _ := s.Load()
	if in, _ := FindIntegration(w, "acme"); in.AWSSSO.TokenExpires != nil {
		t.Fatal("TokenExpires must be cleared on logout")
	}
}

func TestDiscoverReadsFakeHome(t *testing.T) {
	home := fakeHome(t)
	writeFile(t, filepath.Join(home, ".aws", "config"), `[sso-session acme]
sso_start_url = `+portalURL+`
sso_region = eu-west-1

[profile dev]
sso_session = acme
sso_account_id = 1
sso_role_name = Dev

[profile plain]
region = us-east-1
`)
	writeCLITokenCache(t, home, time.Now().Add(time.Hour))
	writeFile(t, filepath.Join(home, ".azure", "azureProfile.json"), "\uFEFF"+`{"subscriptions":[{"tenantId":"t1","user":{"name":"me@contoso.example"}},{"tenantId":"t1"}]}`)
	s := testService(t)
	res := s.Discover(context.Background())
	if len(res.AWSPortals) != 1 {
		t.Fatalf("portals = %+v", res.AWSPortals)
	}
	p := res.AWSPortals[0]
	if p.Alias != "acme" || p.StartURL != portalURL || p.Region != "eu-west-1" || !p.HasToken || p.Source != "aws-cli" || strings.Join(p.Profiles, ",") != "dev" {
		t.Fatalf("portal = %+v", p)
	}
	if len(res.AzureTenants) != 1 || res.AzureTenants[0].TenantID != "t1" || res.AzureTenants[0].Account != "me@contoso.example" {
		t.Fatalf("tenants = %+v", res.AzureTenants)
	}
	if res.GCP != nil || res.Leapp != nil {
		t.Fatalf("unexpected gcp/leapp findings: %+v %+v", res.GCP, res.Leapp)
	}
}

func TestDiscoverEmptyHome(t *testing.T) {
	fakeHome(t)
	res := testService(t).Discover(context.Background())
	if len(res.AWSPortals) != 0 || len(res.AzureTenants) != 0 || res.GCP != nil || res.Leapp != nil {
		t.Fatalf("result = %+v", res)
	}
}

// cacheCreds marks a session active with cached credentials, without a provider call.
func cacheCreds(t *testing.T, s *Service, sess core.Session, creds core.Credentials) {
	t.Helper()
	exp := time.Now().Add(time.Hour).UTC()
	creds.Expiration = &exp
	sess.Status, sess.Expires = core.StatusActive, &exp
	addSession(t, s, sess)
	if err := s.Cache.Put(sess.ID, creds); err != nil {
		t.Fatal(err)
	}
}

func TestSessionEnvPerCloud(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	user := addIAMUser(t, s, "dev")
	if _, err := s.SessionEnv(ctx, user.ID); !errors.Is(err, ErrSessionInactive) {
		t.Fatalf("inactive: %v", err)
	}
	if _, err := s.SessionEnv(ctx, "missing"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown: %v", err)
	}
	if _, err := s.Start(ctx, user.ID, StartOptions{}); err != nil {
		t.Fatal(err)
	}
	cacheCreds(t, s, core.Session{ID: "az1", Name: "sub", Kind: core.KindAzure, Azure: &core.AzureSession{SubscriptionID: "sub-1", TenantID: "ten-1"}}, core.Credentials{Token: "az-tok"})
	cacheCreds(t, s, core.Session{ID: "g1", Name: "proj", Kind: core.KindGCP, GCP: &core.GCPSession{ProjectID: "proj-1"}}, core.Credentials{Token: "g-tok"})
	cacheCreds(t, s, core.Session{ID: "g2", Name: "deployer", Kind: core.KindGCP, GCP: &core.GCPSession{ProjectID: "proj-1", ServiceAccount: "sa@proj-1.iam.gserviceaccount.com"}}, core.Credentials{Token: "sa-tok"})

	want := map[string][][2]string{
		user.ID: {
			{"AWS_ACCESS_KEY_ID", "AKIAdev"}, {"AWS_SECRET_ACCESS_KEY", "secret"}, {"AWS_SESSION_TOKEN", ""},
			{"AWS_REGION", "us-east-1"}, {"AWS_DEFAULT_REGION", "us-east-1"},
		},
		"az1": {
			{"AZURE_SUBSCRIPTION_ID", "sub-1"}, {"AZURE_TENANT_ID", "ten-1"}, {"ARM_SUBSCRIPTION_ID", "sub-1"}, {"ARM_TENANT_ID", "ten-1"}, {"AZURE_ACCESS_TOKEN", "az-tok"},
		},
		"g1": {
			{"CLOUDSDK_CORE_PROJECT", "proj-1"}, {"GOOGLE_CLOUD_PROJECT", "proj-1"}, {"CLOUDSDK_AUTH_ACCESS_TOKEN", "g-tok"}, {"GOOGLE_OAUTH_ACCESS_TOKEN", "g-tok"},
		},
		"g2": {
			{"CLOUDSDK_CORE_PROJECT", "proj-1"}, {"GOOGLE_CLOUD_PROJECT", "proj-1"}, {"CLOUDSDK_AUTH_ACCESS_TOKEN", "sa-tok"}, {"GOOGLE_OAUTH_ACCESS_TOKEN", "sa-tok"},
			{"CLOUDSDK_AUTH_IMPERSONATE_SERVICE_ACCOUNT", "sa@proj-1.iam.gserviceaccount.com"},
			{"GOOGLE_APPLICATION_CREDENTIALS", filepath.Join(s.Cache.Dir, "gcp", "g2.json")},
		},
	}
	for ref, vars := range want {
		got, err := s.SessionEnv(ctx, ref)
		if err != nil {
			t.Fatalf("SessionEnv(%s): %v", ref, err)
		}
		if fmt.Sprint(got) != fmt.Sprint(vars) {
			t.Fatalf("SessionEnv(%s) =\n%v\nwant\n%v", ref, got, vars)
		}
	}
	// Terminal env for AWS points at the profile and never exports a secret.
	w, _ := s.Load()
	sess, _ := FindSession(w, user.ID)
	env, err := s.terminalEnv(ctx, w, sess)
	if err != nil || fmt.Sprint(env) != fmt.Sprint([][2]string{{"AWS_PROFILE", "default"}, {"AWS_REGION", "us-east-1"}, {"AWS_DEFAULT_REGION", "us-east-1"}}) {
		t.Fatalf("terminalEnv = %v, %v", env, err)
	}
	az, _ := FindSession(w, "az1")
	if env, err := s.terminalEnv(ctx, w, az); err != nil || env[4][1] != "az-tok" {
		t.Fatalf("azure terminalEnv = %v, %v", env, err)
	}
}

func TestConsoleURLForPerCloud(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	if _, err := s.ConsoleURLFor(ctx, "missing"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown: %v", err)
	}
	in, _ := s.AddAWSSSO("acme", portalURL, "us-east-1")
	addSession(t, s, core.Session{ID: "r1", Name: "Acme/Admin", Kind: core.KindAWSSSORole, IntegrationID: in.ID, Status: core.StatusInactive, AWS: &core.AWSSession{AccountID: "1", RoleName: "Admin"}})
	if _, err := s.ConsoleURLFor(ctx, "r1"); !errors.Is(err, ErrSessionInactive) {
		t.Fatalf("inactive aws role: %v", err)
	}
	user := addIAMUser(t, s, "dev")
	if _, err := s.Start(ctx, user.ID, StartOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConsoleURLFor(ctx, user.ID); err == nil || !strings.Contains(err.Error(), "needs a role") {
		t.Fatalf("iam user: %v", err)
	}
	addSession(t, s, core.Session{ID: "az1", Name: "sub", Kind: core.KindAzure, Status: core.StatusInactive, Azure: &core.AzureSession{SubscriptionID: "sub", TenantID: "ten-1"}})
	if u, err := s.ConsoleURLFor(ctx, "az1"); err != nil || u != "https://portal.azure.com/#@ten-1" {
		t.Fatalf("azure = %q, %v", u, err)
	}
	if _, err := s.ConsoleURL(ctx, "az1"); err == nil || !strings.Contains(err.Error(), "only available for AWS") {
		t.Fatalf("ConsoleURL on azure: %v", err)
	}
	addSession(t, s, core.Session{ID: "g1", Name: "proj", Kind: core.KindGCP, Status: core.StatusInactive, GCP: &core.GCPSession{ProjectID: "proj-1"}})
	if u, err := s.ConsoleURLFor(ctx, "g1"); err != nil || !strings.HasSuffix(u, "?project=proj-1") {
		t.Fatalf("gcp = %q, %v", u, err)
	}
	addSession(t, s, core.Session{ID: "x1", Name: "odd", Kind: core.Kind("bogus"), Status: core.StatusInactive})
	if _, err := s.ConsoleURLFor(ctx, "x1"); err == nil || !strings.Contains(err.Error(), "no console") {
		t.Fatalf("bogus kind: %v", err)
	}
}

func TestGCPImpersonationFilesFollowSession(t *testing.T) {
	home := fakeHome(t)
	adc := filepath.Join(home, "adc.json")
	writeFile(t, adc, `{"type":"authorized_user","client_id":"c","client_secret":"s","refresh_token":"r"}`)
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", adc)
	s := testService(t)
	sess := &core.Session{ID: "g2", Name: "deployer", Kind: core.KindGCP, GCP: &core.GCPSession{ProjectID: "p", ServiceAccount: "sa@p.iam.gserviceaccount.com"}}
	if err := s.writeCloudFiles(sess); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.Cache.Dir, "gcp", "g2.json")
	data := readFile(t, path)
	if !strings.Contains(data, `"impersonated_service_account"`) || !strings.Contains(data, "sa@p.iam.gserviceaccount.com") || !strings.Contains(data, `"refresh_token": "r"`) {
		t.Fatalf("adc file:\n%s", data)
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %o, want 600", info.Mode().Perm())
		}
	}
	if got := s.EnvVars(sess, core.Credentials{Token: "tok"}); got[len(got)-1] != [2]string{"GOOGLE_APPLICATION_CREDENTIALS", path} {
		t.Fatalf("env = %v", got)
	}
	plain := &core.Session{ID: "g1", Name: "proj", Kind: core.KindGCP, GCP: &core.GCPSession{ProjectID: "p"}}
	if err := s.writeCloudFiles(plain); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.Cache.Dir, "gcp", "g1.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a user-identity session must not write an ADC file")
	}
	if err := s.removeCloudFiles(sess); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("adc file not removed")
	}
	if err := s.removeCloudFiles(sess); err != nil {
		t.Fatalf("removing twice must be harmless: %v", err)
	}
	// Without source credentials the file cannot be written.
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", filepath.Join(home, "missing.json"))
	if err := s.writeCloudFiles(sess); err == nil {
		t.Fatal("expected an error without ADC")
	}
}

func TestAzureLogoutAndRemoveStopSessions(t *testing.T) {
	s := testService(t)
	in, err := s.AddAzure("contoso", "ten-1")
	if err != nil {
		t.Fatal(err)
	}
	cacheCreds(t, s, core.Session{ID: "az1", Name: "sub", Kind: core.KindAzure, IntegrationID: in.ID, Azure: &core.AzureSession{SubscriptionID: "sub", TenantID: "ten-1"}}, core.Credentials{Token: "az-tok"})
	w, _ := s.Load()
	w.Integrations[0].Azure.Account = "me@contoso.example"
	if err := s.Save(w); err != nil {
		t.Fatal(err)
	}
	if err := s.AzureLogout(context.Background(), "contoso"); err != nil {
		t.Fatal(err)
	}
	w, _ = s.Load()
	if w.Integrations[0].Azure.Account != "" || w.Sessions[0].Status != core.StatusInactive {
		t.Fatalf("workspace after logout = %+v", w)
	}
	if _, err := s.Cache.Get("az1"); err == nil {
		t.Fatal("cached token must be dropped")
	}
	if _, err := s.AzureLogin(context.Background(), "missing"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown tenant: %v", err)
	}
	if _, err := s.AddAWSSSO("acme", portalURL, "us-east-1"); err != nil {
		t.Fatal(err)
	}
	if err := s.AzureLogout(context.Background(), "acme"); err == nil || !strings.Contains(err.Error(), "not an Azure tenant") {
		t.Fatalf("logout through an AWS integration: %v", err)
	}
	if err := s.SSOLogout("contoso"); err == nil || !strings.Contains(err.Error(), "not an AWS IAM Identity Center portal") {
		t.Fatalf("sso logout through an Azure integration: %v", err)
	}
	if _, err := s.SyncGCP(context.Background(), "contoso"); err == nil || !strings.Contains(err.Error(), "not a Google Cloud account") {
		t.Fatalf("gcp sync through an Azure integration: %v", err)
	}
	if err := s.RemoveIntegration("contoso"); err != nil {
		t.Fatal(err)
	}
	w, _ = s.Load()
	if len(w.Integrations) != 1 || len(w.Sessions) != 0 {
		t.Fatalf("workspace after remove = %+v", w)
	}
}

func TestImportLeappSessionsNil(t *testing.T) {
	s := testService(t)
	res, err := s.ImportLeappSessions(nil)
	if err != nil || len(res.Sessions) != 0 || len(res.Skipped) != 0 {
		t.Fatalf("nil import = %+v, %v", res, err)
	}
}

func TestImportIAMUserFromCredentialsFile(t *testing.T) {
	home := fakeHome(t)
	s := testService(t)
	credPath := filepath.Join(home, ".aws", "credentials")
	writeFile(t, credPath, "[default]\naws_access_key_id = AKIA1\naws_secret_access_key = s1\n\n[personal]\naws_access_key_id = AKIA2\naws_secret_access_key = s2\n")
	writeFile(t, s.AWSConfigPath, "[profile personal]\nregion = us-west-2\nmfa_serial = arn:aws:iam::1:mfa/me\n")

	sess, err := s.ImportIAMUser("personal")
	if err != nil {
		t.Fatal(err)
	}
	if sess.Name != "personal" || sess.Kind != core.KindAWSIAMUser || sess.Region != "us-west-2" || sess.AWS.MFADevice != "arn:aws:iam::1:mfa/me" || sess.AWS.Profile != "personal" {
		t.Fatalf("session = %+v aws = %+v", sess, sess.AWS)
	}
	if key, err := aws.LoadAccessKey(s.Secrets, sess.ID); err != nil || key.AccessKeyID != "AKIA2" || key.SecretAccessKey != "s2" {
		t.Fatalf("key = %+v, %v", key, err)
	}
	// The key stays in the file until the caller removes it.
	if data, _ := os.ReadFile(credPath); !strings.Contains(string(data), "AKIA2") {
		t.Fatalf("credentials file changed:\n%s", data)
	}
	// A second import of the same name is refused; a missing profile too.
	if _, err := s.ImportIAMUser("personal"); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("duplicate = %v", err)
	}
	if _, err := s.ImportIAMUser("nosuch"); err == nil || !strings.Contains(err.Error(), `no access key for profile "nosuch"`) {
		t.Fatalf("missing = %v", err)
	}
	// Discover and StaticProfiles mark the imported key, not the other one.
	r := s.Discover(context.Background())
	if len(r.IAMUsers) != 2 || r.IAMUsers[0].Profile != "default" || r.IAMUsers[0].Imported || !r.IAMUsers[1].Imported {
		t.Fatalf("discover = %+v", r.IAMUsers)
	}
	st := s.StaticProfiles()
	if len(st.Profiles) != 2 || st.Profiles[0].Imported || !st.Profiles[1].Imported {
		t.Fatalf("static profiles = %+v", st.Profiles)
	}
	// The session records its access key ID. A session from before that
	// gets the ID from the keychain once and keeps it.
	w, _ := s.Load()
	imported, _ := FindSession(w, "personal")
	if imported.AWS.AccessKeyID != "AKIA2" {
		t.Fatalf("recorded access key id = %q", imported.AWS.AccessKeyID)
	}
	imported.AWS.AccessKeyID = ""
	if err := s.Save(w); err != nil {
		t.Fatal(err)
	}
	if st := s.StaticProfiles(); !st.Profiles[1].Imported {
		t.Fatalf("imported mark lost without the recorded id: %+v", st.Profiles)
	}
	w, _ = s.Load()
	if again, _ := FindSession(w, "personal"); again.AWS.AccessKeyID != "AKIA2" {
		t.Fatalf("access key id not filled back: %q", again.AWS.AccessKeyID)
	}
	// A profile without a config section takes the default region.
	if got, err := s.ImportIAMUser("default"); err != nil || got.Region != core.DefaultSettings().DefaultRegion || got.AWS.Profile != "default" {
		t.Fatalf("default profile = %+v, %v", got, err)
	}
}

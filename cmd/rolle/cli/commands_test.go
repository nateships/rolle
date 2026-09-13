package cli

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nateships/rolle/internal/app"
	"github.com/nateships/rolle/internal/aws"
	"github.com/nateships/rolle/internal/azure"
	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/debug"
	"github.com/nateships/rolle/internal/gcp"
)

// isolate keeps every command away from the real home, AWS config, ADC
// file, and network retries.
func isolate(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(home, "absent"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(home, "absent"))
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_MAX_ATTEMPTS", "1")
	t.Setenv("AZURE_CONFIG_DIR", filepath.Join(home, ".azure"))
	t.Setenv("CLOUDSDK_CONFIG", filepath.Join(home, ".gcloud"))
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", filepath.Join(home, "absent-adc.json"))
	return home
}

// fakePortal serves the Identity Center OIDC and portal APIs on one server.
// The device login issues portalToken, which the portal then accepts.
type fakePortal struct{ paths []string }

const portalToken = "portal-token"

func (f *fakePortal) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.paths = append(f.paths, r.URL.Path)
	w.Header().Set("Content-Type", "application/json")
	if strings.HasPrefix(r.URL.Path, "/assignment/") || strings.HasPrefix(r.URL.Path, "/federation/") {
		if r.Header.Get("x-amz-sso_bearer_token") != portalToken {
			w.Header().Set("X-Amzn-ErrorType", "UnauthorizedException")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"message":"Session token not found or invalid"}`)
			return
		}
	}
	switch r.URL.Path {
	case "/client/register":
		fmt.Fprint(w, `{"clientId":"cid","clientSecret":"cs","clientIdIssuedAt":1,"clientSecretExpiresAt":9999999999}`)
	case "/device_authorization":
		fmt.Fprint(w, `{"deviceCode":"dev-code","userCode":"ABCD-EFGH","verificationUri":"https://device.sso.example/","verificationUriComplete":"https://device.sso.example/?user_code=ABCD-EFGH","expiresIn":600,"interval":1}`)
	case "/token":
		fmt.Fprintf(w, `{"accessToken":%q,"refreshToken":"rt","expiresIn":3600,"tokenType":"Bearer"}`, portalToken)
	case "/assignment/accounts":
		fmt.Fprint(w, `{"accountList":[{"accountId":"111111111111","accountName":"Acme"}]}`)
	case "/assignment/roles":
		fmt.Fprint(w, `{"roleList":[{"accountId":"111111111111","roleName":"Admin"},{"accountId":"111111111111","roleName":"ReadOnly"}]}`)
	case "/federation/credentials":
		fmt.Fprintf(w, `{"roleCredentials":{"accessKeyId":"ASIAFAKE","secretAccessKey":"fake-secret","sessionToken":"fake-token","expiration":%d}}`, time.Now().Add(time.Hour).UnixMilli())
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func startFakePortal(t *testing.T) *fakePortal {
	t.Helper()
	f := &fakePortal{}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	t.Setenv("AWS_ENDPOINT_URL_SSO", srv.URL)
	t.Setenv("AWS_ENDPOINT_URL_SSO_OIDC", srv.URL)
	return f
}

// addIntegration appends an integration to the workspace as is.
func addIntegration(t *testing.T, s *app.Service, in core.Integration) {
	t.Helper()
	w, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	w.Integrations = append(w.Integrations, in)
	if err := s.Save(w); err != nil {
		t.Fatal(err)
	}
}

func TestIntegrationLoginDeviceFlowDiscoversRoles(t *testing.T) {
	isolate(t)
	portal := startFakePortal(t)
	s := testCLI(t)
	mustRun(t, "integration", "add", "aws-sso", "--alias", "acme", "--start-url", "https://acme.awsapps.com/start", "--region", "us-east-1")

	out := mustRun(t, "integration", "login", "acme", "--no-browser")
	for _, want := range []string{
		"Open https://device.sso.example/?user_code=ABCD-EFGH\nand confirm code ABCD-EFGH\n",
		"Waiting for approval...\n",
		"Logged in. 2 new role(s) discovered.\n",
		"  Acme/Admin\n",
		"  Acme/ReadOnly\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("login output lacks %q:\n%s", want, out)
		}
	}
	if got := strings.Join(portal.paths, " "); !strings.HasPrefix(got, "/client/register /device_authorization /token /assignment/accounts") {
		t.Fatalf("portal calls = %q", got)
	}
	if _, err := s.Secrets.Get("aws-sso-token/" + reloadIntegration(t, s, "acme").ID); err != nil {
		t.Fatalf("token not stored: %v", err)
	}

	row := fields(lines(mustRun(t, "integration", "list"))[1])
	if row[0] != "acme" || strings.Join(row[2:5], " ") != "logged in until" {
		t.Fatalf("list row = %v", row)
	}
	if out := mustRun(t, "integration", "sync", "acme"); strings.TrimSpace(out) != "0 new session(s)" {
		t.Fatalf("sync output = %q", out)
	}
	// Static keys in the credentials file shadow the profile: start refuses
	// and names the fix, the listing shows the file, and fix-profile clears it.
	credPath := filepath.Join(t.TempDir(), "credentials")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credPath)
	if err := os.WriteFile(credPath, []byte("[default]\naws_access_key_id = AKIA\naws_secret_access_key = x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "start", "Acme/Admin"); err == nil || !strings.Contains(err.Error(), "static keys") || !strings.Contains(err.Error(), "fix-profile") {
		t.Fatalf("start with shadowing keys: %v", err)
	}
	var shadowed []map[string]any
	if err := json.Unmarshal([]byte(mustRun(t, "session", "list", "--json")), &shadowed); err != nil || shadowed[0]["shadowedBy"] != credPath {
		t.Fatalf("shadowedBy = %v, %v", shadowed[0]["shadowedBy"], err)
	}
	if out := mustRun(t, "cleanup"); !strings.Contains(out, credPath) || !strings.Contains(out, "  default\n") {
		t.Fatalf("cleanup list: %q", out)
	}
	if out := mustRun(t, "session", "fix-profile", "Acme/Admin"); !strings.Contains(out, "removed the static keys of profile default") {
		t.Fatalf("fix-profile output: %q", out)
	}
	if out := mustRun(t, "session", "fix-profile", "Acme/Admin"); !strings.Contains(out, "not shadowed") {
		t.Fatalf("second fix-profile output: %q", out)
	}
	if err := os.WriteFile(credPath, []byte("[a]\naws_access_key_id = A\naws_secret_access_key = B\n[b]\naws_access_key_id = C\naws_secret_access_key = D\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var cleanup map[string]any
	if err := json.Unmarshal([]byte(mustRun(t, "cleanup", "--json")), &cleanup); err != nil || !strings.Contains(fmt.Sprint(cleanup["profiles"]), "name:a]") || !strings.Contains(fmt.Sprint(cleanup["profiles"]), "preview:…") {
		t.Fatalf("cleanup --json = %v, %v", cleanup, err)
	}
	if _, err := run(t, "cleanup", "nosuch"); err == nil || !strings.Contains(err.Error(), `no static keys for profile "nosuch"`) {
		t.Fatalf("cleanup of a missing section: %v", err)
	}
	if _, err := run(t, "cleanup", "a", "--all"); err == nil || !strings.Contains(err.Error(), "--all takes no profile names") {
		t.Fatalf("cleanup with names and --all: %v", err)
	}
	mustRun(t, "cleanup", "--all")
	if out := mustRun(t, "cleanup"); !strings.Contains(out, "no static keys") {
		t.Fatalf("cleanup after --all: %q", out)
	}
	out = mustRun(t, "start", "Acme/Admin")
	if !strings.HasPrefix(out, "Acme/Admin active until ") || !strings.Contains(out, "AWS profile: default\n") {
		t.Fatalf("start output:\n%s", out)
	}
	out = mustRun(t, "env", "Acme/Admin")
	if !strings.Contains(out, "export AWS_SESSION_TOKEN='fake-token'\n") {
		t.Fatalf("env output:\n%s", out)
	}

	// --json gives scripts the same facts as the table, without credentials.
	var listed []map[string]any
	if err := json.Unmarshal([]byte(mustRun(t, "session", "list", "--json")), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || listed[0]["name"] != "Acme/Admin" || listed[0]["profile"] != "default" || listed[0]["integration"] != "acme" || listed[0]["status"] != "active" {
		t.Fatalf("session list --json = %v", listed)
	}
	for _, s := range listed {
		for k := range s {
			if strings.Contains(strings.ToLower(k), "secret") || strings.Contains(strings.ToLower(k), "token") {
				t.Fatalf("session list --json leaks %q", k)
			}
		}
	}
	var started map[string]any
	if err := json.Unmarshal([]byte(mustRun(t, "start", "Acme/ReadOnly", "--json")), &started); err != nil {
		t.Fatal(err)
	}
	if started["name"] != "Acme/ReadOnly" || started["status"] != "active" || started["expires"] == nil {
		t.Fatalf("start --json = %v", started)
	}
	var integs []map[string]any
	if err := json.Unmarshal([]byte(mustRun(t, "integration", "list", "--json")), &integs); err != nil {
		t.Fatal(err)
	}
	if len(integs) != 1 || integs[0]["alias"] != "acme" || integs[0]["signedIn"] != true {
		t.Fatalf("integration list --json = %v", integs)
	}
	mustRun(t, "stop", "Acme/ReadOnly")

	// Hidden roles leave the list unless asked for; --account covers the account.
	mustRun(t, "session", "hide", "Acme/ReadOnly")
	if out := mustRun(t, "session", "list"); strings.Contains(out, "ReadOnly") || !strings.Contains(out, "Acme/Admin") {
		t.Fatalf("list after hide:\n%s", out)
	}
	if out := mustRun(t, "session", "list", "--all"); !strings.Contains(out, "ReadOnly") {
		t.Fatalf("list --all after hide:\n%s", out)
	}
	mustRun(t, "session", "hide", "--account", "111111111111")
	if out := mustRun(t, "session", "list"); strings.Contains(out, "Acme/") && !strings.Contains(out, "active") {
		t.Fatalf("list after hiding the account:\n%s", out)
	}
	// Tags group sessions in the sidebar; the CLI manages them too.
	mustRun(t, "tag", "add", "Prod", "--color", "#FF0000", "--icon", "shield")
	mustRun(t, "tag", "add", "Sandbox")
	if _, err := run(t, "tag", "add", "Odd", "--color", "red"); err == nil {
		t.Fatal("a color that is not #rrggbb must fail")
	}
	mustRun(t, "session", "tag", "Acme/Admin", "prod")
	if row := fields(lines(mustRun(t, "tag", "list"))[1]); strings.Join(row, " ") != "Prod #ff0000 shield" {
		t.Fatalf("tag list row = %v", row)
	}
	mustRun(t, "tag", "move", "Sandbox", "0")
	mustRun(t, "tag", "set", "Prod", "--name", "Production", "--icon", "rocket")
	var tags []map[string]any
	if err := json.Unmarshal([]byte(mustRun(t, "tag", "list", "--json")), &tags); err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 || tags[1]["name"] != "Production" || tags[1]["color"] != "#ff0000" || tags[1]["icon"] != "rocket" {
		t.Fatalf("tag list --json = %v", tags)
	}
	// An empty flag value clears the field back to the default.
	mustRun(t, "tag", "set", "Production", "--icon", "")
	if row := fields(lines(mustRun(t, "tag", "list"))[2]); strings.Join(row, " ") != "Production #ff0000" {
		t.Fatalf("tag list row after clearing the icon = %v", row)
	}
	var tagged []map[string]any
	if err := json.Unmarshal([]byte(mustRun(t, "session", "list", "--json", "--all")), &tagged); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(tagged[0]["tags"]) != "[Production]" {
		t.Fatalf("session tags after rename = %v", tagged[0]["tags"])
	}
	mustRun(t, "session", "untag", "Acme/Admin", "Production")
	mustRun(t, "tag", "remove", "Sandbox")
	if out := mustRun(t, "tag", "list"); strings.Contains(out, "Sandbox") || !strings.Contains(out, "Production") {
		t.Fatalf("tag list after remove = %q", out)
	}
	if _, err := run(t, "tag", "move", "Production", "x"); err == nil {
		t.Fatal("non-numeric index must fail")
	}

	mustRun(t, "session", "unhide", "--all")
	if out := mustRun(t, "session", "list"); !strings.Contains(out, "ReadOnly") || !strings.Contains(out, "Acme/Admin") {
		t.Fatalf("list after unhide --all:\n%s", out)
	}
	if _, err := run(t, "session", "unhide"); err == nil {
		t.Fatal("unhide without a target must fail")
	}
	if _, err := run(t, "session", "hide", "--account", "000000000000"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("hiding an unknown account = %v, want not found", err)
	}

	mustRun(t, "integration", "logout", "acme")
	if got := reload(t, s, "Acme/Admin"); got.Status != core.StatusInactive {
		t.Fatalf("logout must stop the session: %+v", got)
	}
	if row := fields(lines(mustRun(t, "int", "list"))[1]); strings.Join(row[2:4], " ") != "logged out" {
		t.Fatalf("list row after logout = %v", row)
	}
	if _, err := run(t, "start", "Acme/Admin"); !errors.Is(err, aws.ErrSSOLoginRequired) {
		t.Fatalf("start after logout = %v", err)
	}
	// A sync with no valid token runs the sign-in instead of failing.
	out = mustRun(t, "integration", "sync", "acme", "--no-browser")
	for _, want := range []string{"acme needs a sign-in.\n", "and confirm code ABCD-EFGH\n", "Logged in. 0 new role(s) discovered.\n"} {
		if !strings.Contains(out, want) {
			t.Fatalf("sync after logout lacks %q:\n%s", want, out)
		}
	}
	if row := fields(lines(mustRun(t, "int", "list"))[1]); strings.Join(row[2:5], " ") != "logged in until" {
		t.Fatalf("list row after sync = %v", row)
	}
}

func reloadIntegration(t *testing.T, s *app.Service, ref string) *core.Integration {
	t.Helper()
	w, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	in, err := app.FindIntegration(w, ref)
	if err != nil {
		t.Fatal(err)
	}
	return in
}

func TestIntegrationCommandsForAzureAndGCP(t *testing.T) {
	isolate(t)
	s := testCLI(t)
	out := mustRun(t, "integration", "add", "azure", "--alias", "contoso", "--tenant", "t-1")
	if !strings.HasPrefix(out, "added contoso (") || !strings.Contains(out, "next: rolle integration login contoso") {
		t.Fatalf("add azure output:\n%s", out)
	}
	if in := reloadIntegration(t, s, "contoso"); in.Cloud != core.CloudAzure || in.Azure.TenantID != "t-1" {
		t.Fatalf("integration = %+v", in)
	}
	if _, err := run(t, "integration", "add", "azure"); err == nil || !strings.Contains(err.Error(), "required flag") {
		t.Fatalf("add azure without alias = %v", err)
	}
	if _, err := run(t, "integration", "add", "gcp"); !errors.Is(err, gcp.ErrNoADC) {
		t.Fatalf("add gcp without ADC = %v", err)
	}
	addIntegration(t, s, core.Integration{ID: "g1", Alias: "gcp", Cloud: core.CloudGCP, GCP: &core.GCPIntegration{Account: "me@example.com"}})

	got := lines(mustRun(t, "integration", "list"))
	if len(got) != 3 {
		t.Fatalf("list:\n%v", got)
	}
	if row := fields(got[1]); row[1] != "azure" || strings.Join(row[2:4], " ") != "logged out" {
		t.Fatalf("azure row = %v", row)
	}
	if row := fields(got[2]); row[1] != "gcp" || row[2] != "me@example.com" || row[3] != "g1" {
		t.Fatalf("gcp row = %v", row)
	}

	// Sign-in state for Azure shows the account.
	w, _ := s.Load()
	w.Integrations[0].Azure.Account = "nate@contoso.com"
	if err := s.Save(w); err != nil {
		t.Fatal(err)
	}
	if row := fields(lines(mustRun(t, "integration", "list"))[1]); row[2] != "nate@contoso.com" {
		t.Fatalf("azure row with account = %v", row)
	}

	// An Azure sync without a login runs the Microsoft sign-in, which needs the
	// network, so the CLI test stops here for contoso.
	if _, err := run(t, "integration", "sync", "gcp"); !errors.Is(err, gcp.ErrNoADC) {
		t.Fatalf("sync gcp without ADC = %v", err)
	}
	if _, err := run(t, "integration", "login", "gcp"); !errors.Is(err, gcp.ErrNoADC) {
		t.Fatalf("login gcp without ADC = %v", err)
	}
	_, err := run(t, "integration", "logout", "gcp")
	if err == nil || !strings.Contains(err.Error(), "gcp uses the gcloud login") {
		t.Fatalf("logout gcp = %v", err)
	}
	mustRun(t, "integration", "logout", "contoso")
	if in := reloadIntegration(t, s, "contoso"); in.Azure.Account != "" {
		t.Fatalf("logout kept the account: %+v", in)
	}
	for _, sub := range []string{"login", "logout", "sync"} {
		if _, err := run(t, "integration", sub, "ghost"); !errors.Is(err, core.ErrNotFound) {
			t.Fatalf("%s ghost = %v", sub, err)
		}
		if _, err := run(t, "integration", sub); err == nil {
			t.Fatalf("%s without an argument accepted", sub)
		}
	}
}

func TestSessionAddAssumeRoleAndImpersonation(t *testing.T) {
	isolate(t)
	s := testCLI(t)
	mustRun(t, "session", "add", "iam-user", "--name", "dev", "--region", "us-east-1", "--access-key-id", "AKIA", "--secret-access-key", "secret")
	out := mustRun(t, "session", "add", "assume-role", "--name", "admin", "--role-arn", "arn:aws:iam::1:role/Admin", "--source", "dev", "--region", "eu-west-1", "--external-id", "ext", "--profile", "admin")
	if !strings.HasPrefix(out, "added admin (") {
		t.Fatalf("add assume-role output:\n%s", out)
	}
	role := reload(t, s, "admin")
	if role.Kind != core.KindAWSAssumeRole || role.AWS.SourceSessionID != reload(t, s, "dev").ID || role.AWS.ExternalID != "ext" || role.AWS.Profile != "admin" || role.Region != "eu-west-1" {
		t.Fatalf("role = %+v", role)
	}
	if _, err := run(t, "session", "add", "assume-role", "--name", "x", "--role-arn", "arn:x", "--source", "ghost", "--region", "r"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown source = %v", err)
	}
	if _, err := run(t, "session", "add", "assume-role", "--name", "x"); err == nil || !strings.Contains(err.Error(), "required flag") {
		t.Fatalf("missing flags = %v", err)
	}

	if _, err := run(t, "session", "add", "gcp-impersonate", "--name", "deployer", "--project", "p", "--service-account", "sa@p.iam.gserviceaccount.com"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("impersonate without an integration = %v", err)
	}
	addIntegration(t, s, core.Integration{ID: "g1", Alias: "gcp", Cloud: core.CloudGCP, GCP: &core.GCPIntegration{}})
	out = mustRun(t, "session", "add", "gcp-impersonate", "--name", "deployer", "--project", "p", "--service-account", "sa@p.iam.gserviceaccount.com")
	if !strings.HasPrefix(out, "added deployer (") {
		t.Fatalf("impersonate output:\n%s", out)
	}
	if got := reload(t, s, "deployer"); got.Kind != core.KindGCP || got.IntegrationID != "g1" || got.GCP.ServiceAccount != "sa@p.iam.gserviceaccount.com" {
		t.Fatalf("session = %+v", got)
	}
	row := fields(lines(mustRun(t, "session", "list"))[3])
	if row[0] != "deployer" || row[1] != "gcp" || row[2] != "inactive" {
		t.Fatalf("list row = %v", row)
	}
	// Removing the source is refused while the role depends on it.
	if _, err := run(t, "session", "remove", "dev"); err == nil || !strings.Contains(err.Error(), "is the source of") {
		t.Fatalf("remove source = %v", err)
	}
}

func TestSupportWritesRedactedBundle(t *testing.T) {
	home := isolate(t)
	t.Setenv("PATH", t.TempDir())
	s := testCLI(t)
	mustRun(t, "session", "add", "iam-user", "--name", "dev", "--region", "us-east-1", "--access-key-id", "AKIAIOSFODNN7EXAMPLE", "--secret-access-key", "wJalrXUtnFEMI")
	mustRun(t, "integration", "add", "aws-sso", "--alias", "acme", "--start-url", "https://acme.awsapps.com/start", "--region", "us-east-1")
	mustRun(t, "start", "dev")
	debug.Logf("test", "support bundle marker 123456789012")

	out := filepath.Join(home, "reports", "bundle.zip")
	got := mustRun(t, "support", "--out", out)
	if got != out+"\nAttach it to a bug report: https://github.com/nateships/rolle/issues/new/choose\n" {
		t.Fatalf("support output:\n%s", got)
	}
	files := readZip(t, out)
	for _, name := range []string{"info.json", "workspace.json", "aws-config.txt", "clouds.txt", "log.txt", "README.txt"} {
		if _, ok := files[name]; !ok {
			t.Errorf("bundle lacks %s", name)
		}
	}
	if !strings.Contains(files["info.json"], `"app": "cli"`) || !strings.Contains(files["info.json"], `"cacheDir": "`+strings.ReplaceAll(s.Cache.Dir, `\`, `\\`)+`"`) {
		t.Fatalf("info.json:\n%s", files["info.json"])
	}
	if !strings.Contains(files["log.txt"], "support bundle marker <account>") || strings.Contains(files["log.txt"], "123456789012") {
		t.Fatalf("log.txt:\n%s", files["log.txt"])
	}
	if strings.Contains(files["workspace.json"], "acme.awsapps") || !strings.Contains(files["workspace.json"], `"alias": "acme"`) {
		t.Fatalf("workspace.json:\n%s", files["workspace.json"])
	}
	if !strings.Contains(files["aws-config.txt"], "credential_process") {
		t.Fatalf("aws-config.txt:\n%s", files["aws-config.txt"])
	}
	for name, body := range files {
		if strings.Contains(body, "wJalrXUtnFEMI") || strings.Contains(body, "AKIAIOSFODNN7EXAMPLE") {
			t.Fatalf("%s leaks the access key", name)
		}
	}

	// Without --out the bundle goes to Downloads, or the temp dir without one.
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	got = mustRun(t, "support")
	path := lines(got)[0]
	if filepath.Dir(path) != filepath.Clean(os.TempDir()) || !strings.HasPrefix(filepath.Base(path), "rolle-support-") {
		t.Fatalf("default path = %q", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(home, "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "support", "--out", filepath.Join(blocker, "x.zip")); err == nil {
		t.Fatal("expected an error when the bundle cannot be written")
	}
}

func readZip(t *testing.T, path string) map[string]string {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	files := map[string]string{}
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		files[f.Name] = string(b)
	}
	return files
}

// withStdin feeds input to os.Stdin for one command run.
func withStdin(t *testing.T, input string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString(input); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = old; _ = r.Close() })
}

func TestResetAsksForConfirmation(t *testing.T) {
	isolate(t)
	s := testCLI(t)
	mustRun(t, "session", "add", "iam-user", "--name", "dev", "--region", "us-east-1", "--access-key-id", "AKIA", "--secret-access-key", "secret")

	withStdin(t, "n\n")
	if _, err := run(t, "reset"); err == nil || err.Error() != "aborted" {
		t.Fatalf("reset with n = %v", err)
	}
	if got := lines(mustRun(t, "session", "list")); len(got) != 2 {
		t.Fatalf("session removed after an aborted reset: %v", got)
	}

	withStdin(t, "y\n")
	if out, err := run(t, "reset"); err != nil || !strings.HasPrefix(out, "rolle reset.") {
		t.Fatalf("reset with y: %q, %v", out, err)
	}
	if _, err := os.Stat(s.WorkspacePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("workspace still present")
	}
}

func TestDebugFlagEnablesDiagnostics(t *testing.T) {
	isolate(t)
	t.Setenv("ROLLE_DEBUG", "")
	debug.Set(false)
	t.Cleanup(func() { debug.Set(false); debugFlag = false })
	testCLI(t)
	mustRun(t, "status")
	if debug.Enabled() {
		t.Fatal("debug on without the flag")
	}
	mustRun(t, "--debug", "status")
	if !debug.Enabled() {
		t.Fatal("--debug did not enable diagnostics")
	}
}

func TestCommandArgumentGuards(t *testing.T) {
	isolate(t)
	testCLI(t)
	mustRun(t, "session", "add", "iam-user", "--name", "dev", "--region", "us-east-1", "--access-key-id", "AKIA", "--secret-access-key", "secret")
	cases := []struct {
		args []string
		want error
	}{
		{[]string{"stop", "ghost"}, core.ErrNotFound},
		{[]string{"env", "ghost"}, core.ErrNotFound},
		{[]string{"shell", "ghost"}, core.ErrNotFound},
		{[]string{"shell", "dev"}, app.ErrSessionInactive},
		{[]string{"session", "region", "ghost", "us-east-1"}, core.ErrNotFound},
		{[]string{"session", "profile", "ghost"}, core.ErrNotFound},
	}
	for _, tc := range cases {
		if _, err := run(t, tc.args...); !errors.Is(err, tc.want) {
			t.Errorf("rolle %s = %v, want %v", strings.Join(tc.args, " "), err, tc.want)
		}
	}
	for _, args := range [][]string{{"creds"}, {"stop"}, {"env"}, {"shell"}, {"console"}, {"session", "remove"}, {"integration", "remove"}, {"start", "a", "b"}} {
		if _, err := run(t, args...); err == nil {
			t.Errorf("rolle %s accepted bad arguments", strings.Join(args, " "))
		}
	}
	if _, err := run(t, "session", "region", "dev", " "); err == nil || !strings.Contains(err.Error(), "region cannot be empty") {
		t.Fatalf("blank region = %v", err)
	}
}

func TestStatusListsOnlyActiveSessions(t *testing.T) {
	isolate(t)
	s := testCLI(t)
	exp := time.Now().Add(time.Hour).UTC()
	w, _ := s.Load()
	w.Sessions = append(w.Sessions,
		core.Session{ID: "az1", Name: "prod", Kind: core.KindAzure, Status: core.StatusActive, Expires: &exp, Azure: &core.AzureSession{SubscriptionID: "s", TenantID: "t"}},
		core.Session{ID: "az2", Name: "dev", Kind: core.KindAzure, Status: core.StatusInactive, Azure: &core.AzureSession{SubscriptionID: "s2", TenantID: "t"}},
	)
	if err := s.Save(w); err != nil {
		t.Fatal(err)
	}
	if err := s.Cache.Put("az1", core.Credentials{Token: "tok", Expiration: &exp}); err != nil {
		t.Fatal(err)
	}
	got := lines(mustRun(t, "status"))
	if len(got) != 2 || fields(got[1])[0] != "prod" {
		t.Fatalf("status:\n%v", got)
	}
	out := mustRun(t, "env", "prod", "--powershell")
	if !strings.Contains(out, "$env:AZURE_ACCESS_TOKEN = 'tok'\n") || !strings.Contains(out, "$env:ARM_TENANT_ID = 't'\n") {
		t.Fatalf("env output:\n%s", out)
	}
}

func TestExitCodes(t *testing.T) {
	cases := map[error]int{
		nil:                                   0,
		errors.New("boom"):                    ExitError,
		aws.ErrSSOLoginRequired:               ExitLoginRequired,
		azure.ErrLoginRequired:                ExitLoginRequired,
		gcp.ErrNoADC:                          ExitLoginRequired,
		core.ErrNotFound:                      ExitNotFound,
		fmt.Errorf("x: %w", core.ErrNotFound): ExitNotFound,
	}
	for err, want := range cases {
		if got := ExitCode(err); got != want {
			t.Errorf("ExitCode(%v) = %d, want %d", err, got, want)
		}
	}
}

func TestSessionAddIAMUserFromProfile(t *testing.T) {
	home := isolate(t)
	s := testCLI(t)
	credPath := filepath.Join(home, "credentials")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credPath)
	if err := os.WriteFile(credPath, []byte("[personal]\naws_access_key_id = AKIA\naws_secret_access_key = secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(s.AWSConfigPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.AWSConfigPath, []byte("[profile personal]\nregion = us-west-2\nmfa_serial = arn:aws:iam::1:mfa/me\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out := mustRun(t, "session", "add", "iam-user", "--from-profile", "personal"); !strings.HasPrefix(out, "added personal (") {
		t.Fatalf("output:\n%s", out)
	}
	var listed []map[string]any
	if err := json.Unmarshal([]byte(mustRun(t, "session", "list", "--json")), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0]["name"] != "personal" || listed[0]["kind"] != "aws-iam-user" || listed[0]["region"] != "us-west-2" || listed[0]["profile"] != "personal" {
		t.Fatalf("session list --json = %v", listed)
	}
	if sess := reload(t, s, "personal"); sess.AWS.MFADevice != "arn:aws:iam::1:mfa/me" {
		t.Fatalf("mfa = %q", sess.AWS.MFADevice)
	}
	// Flags override what the file gives.
	mustRun(t, "session", "add", "iam-user", "--from-profile", "personal", "--name", "work", "--region", "eu-west-1", "--profile", "work")
	if sess := reload(t, s, "work"); sess.Region != "eu-west-1" || sess.AWS.Profile != "work" || sess.AWS.MFADevice != "arn:aws:iam::1:mfa/me" {
		t.Fatalf("overridden = %+v aws = %+v", sess, sess.AWS)
	}
	if _, err := run(t, "session", "add", "iam-user", "--from-profile", "personal", "--access-key-id", "AKIA"); err == nil || !strings.Contains(err.Error(), "none of the others can be") {
		t.Fatalf("conflict = %v", err)
	}
	// The file holds the secret; a flag for it would be dropped, so it is refused.
	if _, err := run(t, "session", "add", "iam-user", "--from-profile", "personal", "--secret-access-key", "s"); err == nil || !strings.Contains(err.Error(), "none of the others can be") {
		t.Fatalf("secret conflict = %v", err)
	}
	if _, err := run(t, "session", "add", "iam-user", "--name", "x", "--region", "r"); err == nil || !strings.Contains(err.Error(), "at least one of the flags") {
		t.Fatalf("neither flag = %v", err)
	}
	if _, err := run(t, "session", "add", "iam-user", "--access-key-id", "AKIA", "--secret-access-key", "s"); err == nil || !strings.Contains(err.Error(), "--name and --region are required") {
		t.Fatalf("missing name = %v", err)
	}
	if _, err := run(t, "session", "add", "iam-user", "--from-profile", "nosuch"); err == nil || !strings.Contains(err.Error(), `no access key for profile "nosuch"`) {
		t.Fatalf("missing profile = %v", err)
	}
}

package discover

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// fakeHome points every home-relative lookup at a fresh directory.
func fakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", home)
	return home
}

func mkdir(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func rfc3339(d time.Duration) string { return time.Now().Add(d).UTC().Format(time.RFC3339) }

func TestAWSPortalsGrantedProfileMarksSharedPortal(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	write(t, cfg, `[sso-session acme]
sso_start_url = https://acme.awsapps.com/start
sso_region = eu-west-1

[profile via-granted]
sso_session = acme
credential_process = granted credential-process --profile via-granted

[profile legacy-same-portal]
sso_start_url = https://acme.awsapps.com/start/
sso_region = eu-west-1
`)
	got := awsPortals(cfg, t.TempDir())
	if len(got) != 1 {
		t.Fatalf("portals = %+v", got)
	}
	p := got[0]
	if p.Source != "granted" || p.Alias != "acme" || p.Region != "eu-west-1" || p.HasToken {
		t.Fatalf("portal = %+v", p)
	}
	if !reflect.DeepEqual(p.Profiles, []string{"legacy-same-portal", "via-granted"}) {
		t.Fatalf("profiles = %v", p.Profiles)
	}
}

func TestAWSPortalsDedupesStartURLVariants(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config")
	write(t, cfg, `[sso-session hash]
sso_start_url = https://acme.awsapps.com/start/#/
sso_region = us-west-2

[default]
sso_start_url = https://acme.awsapps.com/start

[profile slash]
sso_start_url = https://acme.awsapps.com/start/

[profile spaced]
sso_start_url =   https://acme.awsapps.com/start#/
`)
	got := awsPortals(cfg, t.TempDir())
	if len(got) != 1 {
		t.Fatalf("portals = %+v", got)
	}
	p := got[0]
	if p.StartURL != "https://acme.awsapps.com/start" || p.Alias != "hash" || p.Region != "us-west-2" || p.Source != "aws-cli" {
		t.Fatalf("portal = %+v", p)
	}
	if !reflect.DeepEqual(p.Profiles, []string{"default", "slash", "spaced"}) {
		t.Fatalf("profiles = %v", p.Profiles)
	}
}

func TestAWSPortalsDefaultsAndSkips(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config")
	write(t, cfg, `[sso-session no-url]
sso_region = eu-west-1

[profile dangling]
sso_session = nope

[profile plain]
region = us-east-1

[profile legacy]
sso_start_url = https://d-9876543210.awsapps.com/start
`)
	got := awsPortals(cfg, t.TempDir())
	if len(got) != 1 {
		t.Fatalf("portals = %+v", got)
	}
	p := got[0]
	if p.Alias != "d-9876543210" || p.Region != "us-east-1" || p.HasToken || !reflect.DeepEqual(p.Profiles, []string{"legacy"}) {
		t.Fatalf("portal = %+v", p)
	}
	if got := awsPortals(filepath.Join(t.TempDir(), "missing"), t.TempDir()); got != nil {
		t.Fatalf("missing config = %+v", got)
	}
}

func TestAliasFromURL(t *testing.T) {
	cases := map[string]string{
		"https://acme.awsapps.com/start":         "acme",
		"https://my-org.awsapps.com/start/#/":    "my-org",
		"https://d-1234567890.awsapps.com/start": "d-1234567890",
		"https://localhost/start":                "localhost",
		"not a url":                              "aws",
		"":                                       "aws",
		"https:///start":                         "aws",
	}
	for in, want := range cases {
		if got := aliasFromURL(in); got != want {
			t.Errorf("aliasFromURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCLITokenPicksFreshMatch(t *testing.T) {
	cache := t.TempDir()
	const portal = "https://acme.awsapps.com/start"
	write(t, filepath.Join(cache, "a.json"), `{"startUrl":"`+portal+`","accessToken":"expired","expiresAt":"`+rfc3339(-time.Hour)+`"}`)
	write(t, filepath.Join(cache, "b.json"), `{"startUrl":"https://other.awsapps.com/start","accessToken":"other","expiresAt":"`+rfc3339(time.Hour)+`"}`)
	// botocore client registrations share the directory and have no access token.
	write(t, filepath.Join(cache, "c.json"), `{"clientId":"id","clientSecret":"secret","expiresAt":"`+rfc3339(24*time.Hour)+`"}`)
	write(t, filepath.Join(cache, "d.json"), `not json`)
	write(t, filepath.Join(cache, "e.json"), `{"startUrl":"`+portal+`/","region":"eu-west-1","accessToken":"fresh","expiresAt":"`+rfc3339(time.Hour)+`","clientId":"cid","clientSecret":"cs","refreshToken":"rt"}`)
	write(t, filepath.Join(cache, "f.txt"), `{"startUrl":"`+portal+`","accessToken":"wrong-extension","expiresAt":"`+rfc3339(time.Hour)+`"}`)

	got, ok := CLIToken(cache, portal+"/#/")
	if !ok {
		t.Fatal("no token found")
	}
	if got.AccessToken != "fresh" || got.Region != "eu-west-1" || got.ClientID != "cid" || got.ClientSecret != "cs" || got.RefreshToken != "rt" {
		t.Fatalf("token = %+v", got)
	}
	if _, ok := CLIToken(cache, "https://nobody.awsapps.com/start"); ok {
		t.Fatal("token returned for an unknown portal")
	}
}

func TestCLITokenAppliesTwoMinuteSkew(t *testing.T) {
	cases := []struct {
		name  string
		until time.Duration
		want  bool
	}{
		{"expires in one minute", time.Minute, false},
		{"expires in three minutes", 3 * time.Minute, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cache := t.TempDir()
			write(t, filepath.Join(cache, "t.json"), `{"startUrl":"https://x.awsapps.com/start","accessToken":"tok","expiresAt":"`+rfc3339(tc.until)+`"}`)
			if _, ok := CLIToken(cache, "https://x.awsapps.com/start"); ok != tc.want {
				t.Fatalf("ok = %v, want %v", ok, tc.want)
			}
		})
	}
}

func TestCLITokenMissingDir(t *testing.T) {
	if _, ok := CLIToken(filepath.Join(t.TempDir(), "absent"), "https://x.awsapps.com/start"); ok {
		t.Fatal("token found in a missing directory")
	}
}

func TestAWSCLITokenForUsesHomeCache(t *testing.T) {
	home := fakeHome(t)
	cache := mkdir(t, filepath.Join(home, ".aws", "sso", "cache"))
	if got := ssoCacheDir(); got != cache {
		t.Fatalf("ssoCacheDir = %q, want %q", got, cache)
	}
	write(t, filepath.Join(cache, "x.json"), `{"startUrl":"https://acme.awsapps.com/start","accessToken":"tok","expiresAt":"`+rfc3339(time.Hour)+`"}`)
	got, ok := AWSCLITokenFor("https://acme.awsapps.com/start/#/")
	if !ok || got.AccessToken != "tok" {
		t.Fatalf("AWSCLITokenFor = %+v, %v", got, ok)
	}
	if _, ok := AWSCLITokenFor("https://other.awsapps.com/start"); ok {
		t.Fatal("token returned for another portal")
	}
}

func TestAzureProfilePath(t *testing.T) {
	home := fakeHome(t)
	t.Setenv("AZURE_CONFIG_DIR", "")
	if got := azureProfilePath(); got != filepath.Join(home, ".azure", "azureProfile.json") {
		t.Fatalf("azureProfilePath = %q", got)
	}
	custom := t.TempDir()
	t.Setenv("AZURE_CONFIG_DIR", custom)
	if got := azureProfilePath(); got != filepath.Join(custom, "azureProfile.json") {
		t.Fatalf("azureProfilePath with AZURE_CONFIG_DIR = %q", got)
	}
}

func TestAzureTenantsIgnoresCorruptAndEmpty(t *testing.T) {
	p := filepath.Join(t.TempDir(), "azureProfile.json")
	write(t, p, "{bad")
	if got := azureTenants(p); got != nil {
		t.Fatalf("corrupt profile = %+v", got)
	}
	write(t, p, `{"subscriptions":[{"tenantId":"","user":{"name":"me"}}]}`)
	if got := azureTenants(p); len(got) != 0 {
		t.Fatalf("empty tenant ids = %+v", got)
	}
	write(t, p, `{"subscriptions":[{"tenantId":"zz"},{"tenantId":"aa","user":{"name":"a@x"}}]}`)
	got := azureTenants(p)
	if len(got) != 2 || got[0].TenantID != "aa" || got[1].TenantID != "zz" || got[0].Source != "" {
		t.Fatalf("tenants = %+v", got)
	}
}

func TestMergeLeapp(t *testing.T) {
	base := func() Result {
		return Result{
			AWSPortals:   []AWSPortal{{StartURL: "https://a.awsapps.com/start", Source: "aws-cli"}},
			AzureTenants: []AzureTenant{{TenantID: "t1", Source: "az"}},
		}
	}
	r := base()
	r.mergeLeapp(&LeappWorkspace{
		Portals: []AWSPortal{{StartURL: "https://a.awsapps.com/start", Source: "leapp"}, {StartURL: "https://b.awsapps.com/start", Source: "leapp"}},
		Tenants: []AzureTenant{{TenantID: "t1", Source: "leapp"}, {TenantID: "t2", Source: "leapp"}},
	})
	if len(r.AWSPortals) != 2 || r.AWSPortals[0].Source != "aws-cli" || r.AWSPortals[1].StartURL != "https://b.awsapps.com/start" {
		t.Fatalf("portals = %+v", r.AWSPortals)
	}
	if len(r.AzureTenants) != 2 || r.AzureTenants[0].Source != "az" || r.AzureTenants[1].TenantID != "t2" {
		t.Fatalf("tenants = %+v", r.AzureTenants)
	}
	if r.Leapp != nil {
		t.Fatal("Leapp set without importable sessions")
	}

	cases := map[string]*LeappWorkspace{
		"sso roles":     {SSORoles: 1},
		"iam users":     {IAMUsers: []LeappIAMUser{{ID: "u1"}}},
		"chained roles": {ChainedRoles: []LeappChainedRole{{Name: "c"}}},
	}
	for name, lw := range cases {
		r := base()
		r.mergeLeapp(lw)
		if r.Leapp != lw {
			t.Errorf("%s: Leapp not attached", name)
		}
	}
}

func TestScanUsesOverriddenLocations(t *testing.T) {
	home := fakeHome(t)
	cfg := filepath.Join(t.TempDir(), "config")
	write(t, cfg, "[sso-session acme]\nsso_start_url = https://acme.awsapps.com/start\nsso_region = eu-west-1\n\n[profile dev]\nsso_session = acme\n\n[profile keys]\nregion = us-west-2\nmfa_serial = arn:aws:iam::1:mfa/me\n")
	t.Setenv("AWS_CONFIG_FILE", cfg)
	creds := filepath.Join(t.TempDir(), "credentials")
	write(t, creds, "[keys]\naws_access_key_id = AKIA\naws_secret_access_key = secret\n")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", creds)
	cache := mkdir(t, filepath.Join(home, ".aws", "sso", "cache"))
	write(t, filepath.Join(cache, "x.json"), `{"startUrl":"https://acme.awsapps.com/start/","accessToken":"tok","expiresAt":"`+rfc3339(time.Hour)+`"}`)

	azDir := t.TempDir()
	write(t, filepath.Join(azDir, "azureProfile.json"), `{"subscriptions":[{"tenantId":"t1","user":{"name":"me@contoso.com"}}]}`)
	t.Setenv("AZURE_CONFIG_DIR", azDir)

	adc := filepath.Join(t.TempDir(), "adc.json")
	write(t, adc, `{"type":"authorized_user","client_id":"c","client_secret":"s","refresh_token":"r","account":"me@example.com"}`)
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", adc)
	t.Setenv("LEAPP_HOME", t.TempDir())

	r := Scan(context.Background())
	if len(r.AWSPortals) != 1 || !r.AWSPortals[0].HasToken || r.AWSPortals[0].Alias != "acme" || !reflect.DeepEqual(r.AWSPortals[0].Profiles, []string{"dev"}) {
		t.Fatalf("portals = %+v", r.AWSPortals)
	}
	if len(r.AzureTenants) != 1 || r.AzureTenants[0].TenantID != "t1" || r.AzureTenants[0].Account != "me@contoso.com" {
		t.Fatalf("tenants = %+v", r.AzureTenants)
	}
	if r.GCP == nil || r.GCP.Account != "me@example.com" {
		t.Fatalf("gcp = %+v", r.GCP)
	}
	if r.Leapp != nil {
		t.Fatalf("leapp = %+v", r.Leapp)
	}
	// The key comes without its secret; region and MFA come from the config section.
	want := IAMUserKey{Profile: "keys", AccessKeyID: "AKIA", Region: "us-west-2", MFADevice: "arn:aws:iam::1:mfa/me"}
	if len(r.IAMUsers) != 1 || r.IAMUsers[0] != want {
		t.Fatalf("iam users = %+v", r.IAMUsers)
	}
}

func TestScanWithNothingInstalled(t *testing.T) {
	home := fakeHome(t)
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(home, ".aws", "config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(home, ".aws", "credentials"))
	t.Setenv("AZURE_CONFIG_DIR", filepath.Join(home, ".azure"))
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", filepath.Join(home, "no-adc.json"))
	t.Setenv("LEAPP_HOME", home)
	r := Scan(context.Background())
	if r.AWSPortals != nil || r.AzureTenants != nil || r.IAMUsers != nil || r.GCP != nil || r.Leapp != nil {
		t.Fatalf("empty machine = %+v", r)
	}
	// Without the ADC override the default path under the fake home is used.
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	if r := Scan(context.Background()); r.GCP != nil {
		t.Fatalf("gcp found without an ADC file: %+v", r.GCP)
	}
}

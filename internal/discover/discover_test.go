package discover

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func write(t *testing.T, path string, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestAWSPortalsGroupsSessionsAndLegacyProfiles(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	write(t, cfg, `[sso-session acme]
sso_start_url = https://acme.awsapps.com/start
sso_region = eu-west-1

[profile dev]
sso_session = acme
sso_account_id = 1

[profile prod]
sso_session = acme

[profile legacy]
sso_start_url = https://d-1234567890.awsapps.com/start
sso_region = us-east-1
sso_account_id = 2

[sso-session acme-slash]
sso_start_url = https://acme.awsapps.com/start/
sso_region = eu-west-1

[profile plain]
region = us-east-1

[profile granted-prod]
granted_sso_start_url = https://granted.awsapps.com/start
granted_sso_region = us-west-2
granted_sso_account_id = 3
granted_sso_role_name = Admin
credential_process = granted credential-process --profile granted-prod
`)
	cache := filepath.Join(dir, "sso", "cache")
	if err := os.MkdirAll(cache, 0o700); err != nil {
		t.Fatal(err)
	}
	exp := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	write(t, filepath.Join(cache, "abc.json"), `{"startUrl":"https://acme.awsapps.com/start","region":"eu-west-1","accessToken":"tok","expiresAt":"`+exp+`","clientId":"c","clientSecret":"s"}`)

	got := awsPortals(cfg, cache)
	if len(got) != 3 {
		t.Fatalf("portals = %+v", got)
	}
	var granted *AWSPortal
	for i := range got {
		if got[i].Source == "granted" {
			granted = &got[i]
		}
	}
	if granted == nil || granted.Alias != "granted" || granted.Region != "us-west-2" {
		t.Fatalf("granted portal = %+v", granted)
	}
	acme := got[0]
	if acme.Alias != "acme" || acme.Region != "eu-west-1" || len(acme.Profiles) != 2 || !acme.HasToken || acme.StartURL != "https://acme.awsapps.com/start" {
		t.Fatalf("acme = %+v", acme)
	}
	legacy := got[1]
	if legacy.Alias != "d-1234567890" || legacy.HasToken || legacy.Profiles[0] != "legacy" {
		t.Fatalf("legacy = %+v", legacy)
	}
}

func TestNormalizeStartURL(t *testing.T) {
	want := "https://acme.awsapps.com/start"
	for _, raw := range []string{"https://acme.awsapps.com/start/#/", " https://acme.awsapps.com/start/ ", want} {
		if got := normalizeStartURL(raw); got != want {
			t.Fatalf("normalizeStartURL(%q) = %q", raw, got)
		}
	}
}

func TestCLITokenIgnoresExpired(t *testing.T) {
	cache := t.TempDir()
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	write(t, filepath.Join(cache, "x.json"), `{"startUrl":"https://x.awsapps.com/start","accessToken":"tok","expiresAt":"`+past+`"}`)
	if _, ok := CLIToken(cache, "https://x.awsapps.com/start/"); ok {
		t.Fatal("expired token accepted")
	}
}

func TestAzureTenantsDedupesAndSkipsBOM(t *testing.T) {
	p := filepath.Join(t.TempDir(), "azureProfile.json")
	write(t, p, "\uFEFF"+`{"subscriptions":[{"tenantId":"t1","user":{"name":"me@x.com"}},{"tenantId":"t1"},{"tenantId":"t2","user":{"name":"me@x.com"}}]}`)
	got := azureTenants(p)
	if len(got) != 2 || got[0].TenantID != "t1" || got[0].Account != "me@x.com" {
		t.Fatalf("tenants = %+v", got)
	}
	if azureTenants(filepath.Join(t.TempDir(), "missing.json")) != nil {
		t.Fatal("missing profile should yield nil")
	}
}

func TestLeappRoundTripAndParse(t *testing.T) {
	plain := []byte(`{"_sessions":[
	  {"type":"awsIamUser","sessionName":"personal","sessionId":"u1","region":"us-east-1","profileId":"p1","mfaDevice":"arn:aws:iam::1:mfa/me"},
	  {"type":"awsIamRoleChained","sessionName":"prod-admin","sessionId":"c1","region":"eu-west-1","profileId":"p0","roleArn":"arn:aws:iam::2:role/Admin","parentSessionId":"u1"},
	  {"type":"awsSsoRole","sessionName":"Acme/Admin","sessionId":"s1","region":"us-east-1"},
	  {"type":"azure","sessionName":"Sub","sessionId":"a1","subscriptionId":"sub","tenantId":"ten"}],
	 "_awsSsoIntegrations":[{"alias":"acme","portalUrl":"https://acme.awsapps.com/start/#/","region":"us-east-1"}],
	 "_azureIntegrations":[{"alias":"contoso","tenantId":"ten"}],
	 "_profiles":[{"id":"p0","name":"default"},{"id":"p1","name":"me"}]}`)
	enc := encryptCryptoJS(plain, "machine-secret", []byte("12345678"))
	dec, err := decryptCryptoJS(enc, "machine-secret")
	if err != nil || string(dec) != string(plain) {
		t.Fatalf("round trip failed: %v", err)
	}
	if _, err := decryptCryptoJS(enc, "wrong"); err == nil {
		t.Fatal("wrong passphrase accepted")
	}
	lw, err := parseLeapp(dec)
	if err != nil {
		t.Fatal(err)
	}
	if len(lw.Portals) != 1 || lw.Portals[0].StartURL != "https://acme.awsapps.com/start" || lw.Portals[0].Source != "leapp" {
		t.Fatalf("portals = %+v", lw.Portals)
	}
	if len(lw.Tenants) != 1 || lw.Tenants[0].TenantID != "ten" {
		t.Fatalf("tenants = %+v", lw.Tenants)
	}
	if len(lw.IAMUsers) != 1 || lw.IAMUsers[0].Profile != "me" || lw.IAMUsers[0].MFADevice == "" {
		t.Fatalf("users = %+v", lw.IAMUsers)
	}
	if len(lw.ChainedRoles) != 1 || lw.ChainedRoles[0].ParentName != "personal" || lw.ChainedRoles[0].Profile != "" {
		t.Fatalf("chained = %+v", lw.ChainedRoles)
	}
	if lw.SSORoles != 1 {
		t.Fatalf("sso roles = %d", lw.SSORoles)
	}
}

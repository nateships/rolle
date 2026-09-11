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

[profile plain]
region = us-east-1
`)
	cache := filepath.Join(dir, "sso", "cache")
	if err := os.MkdirAll(cache, 0o700); err != nil {
		t.Fatal(err)
	}
	exp := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	write(t, filepath.Join(cache, "abc.json"), `{"startUrl":"https://acme.awsapps.com/start","region":"eu-west-1","accessToken":"tok","expiresAt":"`+exp+`","clientId":"c","clientSecret":"s"}`)

	got := awsPortals(cfg, cache)
	if len(got) != 2 {
		t.Fatalf("portals = %+v", got)
	}
	acme := got[0]
	if acme.Alias != "acme" || acme.Region != "eu-west-1" || len(acme.Profiles) != 2 || !acme.HasToken {
		t.Fatalf("acme = %+v", acme)
	}
	legacy := got[1]
	if legacy.Alias != "aws" || legacy.HasToken || legacy.Profiles[0] != "legacy" {
		t.Fatalf("legacy = %+v", legacy)
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

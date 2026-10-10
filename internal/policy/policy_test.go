package policy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func trustAll(os.FileInfo) bool { return true }

func TestLoadReadsXMLAndBinaryProfiles(t *testing.T) {
	p := load([]string{"testdata/computer.plist", "testdata/user.plist"}, trustAll)
	if !p.DisableUpdates {
		t.Error("DisableUpdates = false")
	}
	want := map[string]string{
		// The user-level profile comes later and wins.
		"defaultRegion":     `"ap-southeast-2"`,
		"assumeRoleMinutes": `240`,
		"proxyUrl":          `"http://proxy.example.com:3128"`,
		"hiddenSections":    `["gcp"]`,
		"notifyOff":         `true`,
	}
	if len(p.Settings) != len(want) {
		t.Errorf("settings = %v", p.Settings)
	}
	for k, v := range want {
		if got := string(p.Settings[k]); got != v {
			t.Errorf("%s = %s, want %s", k, got, v)
		}
	}
	// Theme is not a key that a profile can set.
	if _, ok := p.Settings["theme"]; ok {
		t.Error("theme accepted")
	}
	slices.Sort(p.Locked)
	if !slices.Equal(p.Locked, []string{"defaultRegion", "proxyUrl"}) {
		t.Errorf("locked = %v", p.Locked)
	}
}

func TestLoadSkipsMissingBrokenAndUntrustedFiles(t *testing.T) {
	if p := load([]string{"testdata/none.plist", "testdata/broken.plist"}, trustAll); !p.Empty() {
		t.Errorf("policy = %+v", p)
	}
	if p := load([]string{"testdata/computer.plist"}, func(os.FileInfo) bool { return false }); !p.Empty() {
		t.Errorf("untrusted policy = %+v", p)
	}
}

func TestLoadSkipsValueOfWrongType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "p.plist")
	body := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict><key>Settings</key><dict>
<key>AssumeRoleMinutes</key><string>sixty</string>
<key>DefaultRegion</key><string>us-west-2</string>
</dict></dict></plist>`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	p := load([]string{path}, trustAll)
	if _, ok := p.Settings["assumeRoleMinutes"]; ok {
		t.Error("string accepted for assumeRoleMinutes")
	}
	if got := string(p.Settings["defaultRegion"]); got != `"us-west-2"` {
		t.Errorf("defaultRegion = %s", got)
	}
}

func TestLockedNeedsValue(t *testing.T) {
	p := Policy{
		Settings: map[string]json.RawMessage{"proxyUrl": json.RawMessage(`"http://p"`)},
		Locked:   []string{"proxyUrl", "caBundle"},
	}
	if !p.IsLocked("proxyUrl") || p.IsLocked("caBundle") {
		t.Errorf("IsLocked: proxyUrl %v, caBundle %v", p.IsLocked("proxyUrl"), p.IsLocked("caBundle"))
	}
}

// No test machine carries a rolle configuration profile.
func TestLoadWithoutProfileIsEmpty(t *testing.T) {
	if p := Load(); !p.Empty() {
		t.Fatalf("policy = %+v", p)
	}
}

func TestLoadReadsIntegrationsAndSkipsBadEntries(t *testing.T) {
	p := load([]string{"testdata/integrations.plist"}, trustAll)
	want := []Integration{
		{Type: "aws-sso", Alias: "acme", StartURL: "https://acme.awsapps.com/start", Region: "us-east-1"},
		{Type: "azure", Alias: "contoso", TenantID: "72f988bf-86f1-41af-91ab-2d7cd011db47"},
	}
	if !slices.Equal(p.Integrations, want) {
		t.Fatalf("integrations = %+v", p.Integrations)
	}
	if p.Empty() {
		t.Error("Empty() = true with integrations")
	}
	if p.Integrations[0].Key() != "aws-sso:https://acme.awsapps.com/start" || p.Integrations[1].Key() != "azure:72f988bf-86f1-41af-91ab-2d7cd011db47" {
		t.Errorf("keys = %q, %q", p.Integrations[0].Key(), p.Integrations[1].Key())
	}
}

// A later file replaces an integration with the same key.
func TestLoadLaterIntegrationWins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "user.plist")
	body := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict><key>Integrations</key><array><dict>
<key>Type</key><string>aws-sso</string><key>Alias</key><string>acme-eu</string>
<key>StartURL</key><string>https://ACME.awsapps.com/start</string><key>Region</key><string>eu-west-1</string>
</dict></array></dict></plist>`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	p := load([]string{"testdata/integrations.plist", path}, trustAll)
	if len(p.Integrations) != 2 || p.Integrations[0].Alias != "acme-eu" || p.Integrations[0].Region != "eu-west-1" {
		t.Fatalf("integrations = %+v", p.Integrations)
	}
}

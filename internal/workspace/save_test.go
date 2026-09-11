package workspace

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nateships/rolle/internal/core"
)

func TestSaveLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "workspace.json")
	if err := Save(path, &core.Workspace{}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "workspace.json" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("dir contains %v, want only workspace.json", names)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(data), "}\n") {
		t.Fatalf("file should end with a newline:\n%s", data)
	}
}

func TestSaveOverwritesAndSetsVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.json")
	first := &core.Workspace{Version: 99, Sessions: []core.Session{{ID: "s1"}}}
	if err := Save(path, first); err != nil {
		t.Fatal(err)
	}
	if first.Version != core.WorkspaceVersion {
		t.Fatalf("Save left Version = %d", first.Version)
	}
	second := &core.Workspace{Sessions: []core.Session{{ID: "s1"}, {ID: "s2"}}}
	if err := Save(path, second); err != nil {
		t.Fatal(err)
	}
	out, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Sessions) != 2 || out.Version != core.WorkspaceVersion {
		t.Fatalf("loaded = %+v", out)
	}
}

func TestSaveLoadPreservesEveryField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.json")
	tokenExp := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	sessExp := tokenExp.Add(time.Hour)
	in := &core.Workspace{
		Integrations: []core.Integration{
			{ID: "i1", Alias: "acme", Cloud: core.CloudAWS, AWSSSO: &core.AWSSSOIntegration{StartURL: "https://acme.awsapps.com/start", Region: "eu-west-1", TokenExpires: &tokenExp}},
			{ID: "i2", Alias: "contoso", Cloud: core.CloudAzure, Azure: &core.AzureIntegration{TenantID: "t", Account: "me@contoso.com"}},
			{ID: "i3", Alias: "google", Cloud: core.CloudGCP, GCP: &core.GCPIntegration{Account: "me@gmail.com"}},
		},
		Sessions: []core.Session{
			{ID: "s1", Name: "prod", Kind: core.KindAWSSSORole, Region: "eu-west-1", IntegrationID: "i1", Status: core.StatusActive, Favorite: true, Expires: &sessExp,
				AWS: &core.AWSSession{Profile: "prod", AccountID: "1", RoleName: "Admin"}},
			{ID: "s2", Name: "chain", Kind: core.KindAWSAssumeRole, Status: core.StatusInactive,
				AWS: &core.AWSSession{RoleARN: "arn:aws:iam::2:role/X", SourceSessionID: "s1", ExternalID: "ext", MFADevice: "arn:mfa"}},
			{ID: "s3", Name: "sub", Kind: core.KindAzure, IntegrationID: "i2", Status: core.StatusInactive, Azure: &core.AzureSession{SubscriptionID: "sub", TenantID: "t"}},
			{ID: "s4", Name: "proj", Kind: core.KindGCP, IntegrationID: "i3", Status: core.StatusInactive, GCP: &core.GCPSession{ProjectID: "p", ServiceAccount: "sa@p.iam.gserviceaccount.com"}},
		},
		Onboarded: true,
		Settings:  &core.Settings{Theme: "dark", DefaultRegion: "eu-west-1", AssumeRoleMinutes: 30, VerboseLogging: true, AutoUpdateOff: true, Terminal: "iterm"},
	}
	if err := Save(path, in); err != nil {
		t.Fatal(err)
	}
	out, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("round trip changed the workspace:\n in: %+v\nout: %+v", in, out)
	}
}

func TestLoadCorruptJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	w, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "parse") || w != nil {
		t.Fatalf("Load = %v, %v", w, err)
	}
}

func TestLoadDirectoryErrors(t *testing.T) {
	if _, err := Load(t.TempDir()); err == nil {
		t.Fatal("expected an error when the path is a directory")
	}
}

func TestLoadUpgradesOlderVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.json")
	if err := os.WriteFile(path, []byte(`{"version":0,"sessions":[{"id":"s1"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	w, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if w.Version != core.WorkspaceVersion || len(w.Sessions) != 1 || w.Settings != nil {
		t.Fatalf("loaded = %+v", w)
	}
}

func TestDefaultPathHonoursEnv(t *testing.T) {
	custom := filepath.Join(t.TempDir(), "ws.json")
	t.Setenv("ROLLE_WORKSPACE", custom)
	if got := DefaultPath(); got != custom {
		t.Fatalf("DefaultPath = %q, want %q", got, custom)
	}
	t.Setenv("ROLLE_WORKSPACE", "")
	if got := DefaultPath(); !strings.HasSuffix(got, filepath.Join("rolle", "workspace.json")) {
		t.Fatalf("DefaultPath = %q, want a rolle/workspace.json path", got)
	}
}

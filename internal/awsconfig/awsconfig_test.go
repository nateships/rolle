package awsconfig

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWriteThenRemoveKeepsForeignProfiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte("[profile mine]\nregion = eu-west-1\nsso_start_url = https://acme.awsapps.com/start/#/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := Write(path, Profile{Name: "prod", Region: "us-east-1", SessionID: "abc", Executable: "/usr/local/bin/rolle"})
	if err != nil {
		t.Fatal(err)
	}
	if err := Write(path, Profile{Name: "mine", SessionID: "abc", Executable: "/usr/local/bin/rolle"}); err == nil {
		t.Fatal("expected refusal to overwrite foreign profile")
	}
	data, _ := os.ReadFile(path)
	s := string(data)
	for _, want := range []string{"[profile prod]", "credential_process = /usr/local/bin/rolle creds --session abc", "rolle_session = abc", "[profile mine]", "sso_start_url = https://acme.awsapps.com/start/#/"} {
		if !strings.Contains(s, want) {
			t.Fatalf("config missing %q:\n%s", want, s)
		}
	}
	if err := Remove(path, "prod", "other"); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); !strings.Contains(string(data), "[profile prod]") {
		t.Fatalf("profile of another session removed:\n%s", data)
	}
	if err := Remove(path, "prod", "abc"); err != nil {
		t.Fatal(err)
	}
	if err := Remove(path, "mine", "abc"); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if strings.Contains(string(data), "prod") || !strings.Contains(string(data), "[profile mine]") {
		t.Fatalf("after remove:\n%s", data)
	}
}

func TestWriteKeepsSymlinkedConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	target := filepath.Join(t.TempDir(), "dotfiles", "aws-config")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("[default]\nregion = eu-west-1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "config")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := Write(link, Profile{Name: "prod", SessionID: "abc", Executable: "/usr/local/bin/rolle"}); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("config is no longer a symlink: %v, %v", fi, err)
	}
	data, _ := os.ReadFile(target)
	if !strings.Contains(string(data), "[profile prod]") || !strings.Contains(string(data), "region = eu-west-1") {
		t.Fatalf("target not updated:\n%s", data)
	}
	if fi, err := os.Stat(target); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("target mode = %v, %v", fi.Mode(), err)
	}
}

func TestQuoteExecutableWithSpaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := Write(path, Profile{Name: "default", SessionID: "x", Executable: "/Applications/rolle.app/Contents/MacOS/rolle cli"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `[default]`) || !strings.Contains(string(data), `"/Applications/rolle.app/Contents/MacOS/rolle cli" creds`) {
		t.Fatalf("config:\n%s", data)
	}
}

func TestWriteTakesOverPlainSectionAndRestoresIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte("[default]\nregion = eu-west-1\noutput = json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, Profile{Name: "default", Region: "us-east-1", SessionID: "s1", Executable: "/opt/rolle"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	for _, want := range []string{"region = us-east-1", "output = json", "credential_process = /opt/rolle creds --session s1", "rolle_preexisting = true", "rolle_previous_region = eu-west-1"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("missing %q:\n%s", want, data)
		}
	}
	if err := Remove(path, "default", "s1"); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	got := string(data)
	if !strings.Contains(got, "region = eu-west-1") || !strings.Contains(got, "output = json") || strings.Contains(got, "rolle") || strings.Contains(got, "credential_process") {
		t.Fatalf("section not restored:\n%s", got)
	}
}

func TestWriteRefusesSectionWithCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte("[default]\nsso_session = acme\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, Profile{Name: "default", SessionID: "s1", Executable: "/opt/rolle"}); err == nil {
		t.Fatal("expected refusal")
	}
}

func TestIAMUserKeysReadsPairsAndConfigSections(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config")
	credPath := filepath.Join(dir, "credentials")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credPath)
	// A missing file is quiet.
	if got := IAMUserKeys(config); got != nil {
		t.Fatalf("missing file = %+v", got)
	}
	creds := "[default]\naws_access_key_id = AKIA1\naws_secret_access_key = s1\n\n[personal]\naws_access_key_id = AKIA2\naws_secret_access_key = s2\n\n[creds-wins]\naws_access_key_id = AKIA4\naws_secret_access_key = s4\nregion = eu-west-1\nmfa_serial = arn:aws:iam::1:mfa/creds\n\n[token-only]\naws_session_token = t\n\n[half]\naws_access_key_id = AKIA3\n"
	if err := os.WriteFile(credPath, []byte(creds), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte("[default]\nregion = us-east-1\nmfa_serial = arn:aws:iam::1:mfa/me\n\n[profile personal]\nregion = us-west-2\n\n[profile creds-wins]\nregion = us-east-1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := IAMUserKeys(config)
	want := []IAMUserKey{
		{Profile: "default", AccessKeyID: "AKIA1", SecretAccessKey: "s1", Region: "us-east-1", MFADevice: "arn:aws:iam::1:mfa/me"},
		{Profile: "personal", AccessKeyID: "AKIA2", SecretAccessKey: "s2", Region: "us-west-2"},
		// The credentials file's own region and mfa_serial win over the config section.
		{Profile: "creds-wins", AccessKeyID: "AKIA4", SecretAccessKey: "s4", Region: "eu-west-1", MFADevice: "arn:aws:iam::1:mfa/creds"},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("keys = %+v", got)
	}
	// Without a config file the keys still come through.
	if err := os.Remove(config); err != nil {
		t.Fatal(err)
	}
	if got := IAMUserKeys(config); len(got) != 3 || got[0].Region != "" || got[0].MFADevice != "" {
		t.Fatalf("keys without config = %+v", got)
	}
	// A config file that does not load, here a directory, costs only the region and MFA device.
	if err := os.Mkdir(config, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := IAMUserKeys(config); len(got) != 3 || got[1].AccessKeyID != "AKIA2" || got[1].Region != "" {
		t.Fatalf("keys with unreadable config = %+v", got)
	}
}

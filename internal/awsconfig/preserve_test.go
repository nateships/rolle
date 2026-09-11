package awsconfig

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func mustWrite(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

var rolle = Profile{Name: "prod", Region: "us-east-1", SessionID: "abc", Executable: "/opt/rolle"}

func TestWriteIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := Write(path, rolle); err != nil {
		t.Fatal(err)
	}
	first := mustRead(t, path)
	if err := Write(path, rolle); err != nil {
		t.Fatal(err)
	}
	second := mustRead(t, path)
	if first != second {
		t.Fatalf("second Write changed the file:\n%s\n---\n%s", first, second)
	}
	if strings.Count(second, "[profile prod]") != 1 || strings.Count(second, "credential_process") != 1 {
		t.Fatalf("duplicate keys or sections:\n%s", second)
	}
}

func TestWriteUpdatesOwnedProfileInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := Write(path, rolle); err != nil {
		t.Fatal(err)
	}
	updated := Profile{Name: "prod", Region: "eu-west-1", SessionID: "xyz", Executable: "/new/rolle"}
	if err := Write(path, updated); err != nil {
		t.Fatal(err)
	}
	got := mustRead(t, path)
	for _, want := range []string{"region = eu-west-1", "rolle_session = xyz", "credential_process = /new/rolle creds --session xyz"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
	for _, stale := range []string{"abc", "us-east-1", "/opt/rolle", "rolle_preexisting"} {
		if strings.Contains(got, stale) {
			t.Errorf("stale %q still present:\n%s", stale, got)
		}
	}
	if strings.Count(got, "[profile prod]") != 1 {
		t.Fatalf("section duplicated:\n%s", got)
	}
}

func TestWriteThenRemoveRestoresForeignContentByteForByte(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	orig := `# top comment
[profile mine]
; keep me
region = eu-west-1
output = json

[sso-session acme]
sso_start_url = https://acme.awsapps.com/start/#/
sso_region = us-east-1

[default]
region = ap-south-1
`
	mustWrite(t, path, orig)
	if err := Write(path, rolle); err != nil {
		t.Fatal(err)
	}
	after := mustRead(t, path)
	if !strings.HasPrefix(after, orig) {
		t.Fatalf("Write reordered or reformatted existing content:\n%s", after)
	}
	if err := Remove(path, "prod", "abc"); err != nil {
		t.Fatal(err)
	}
	if got := mustRead(t, path); got != orig {
		t.Fatalf("Remove did not restore the original file:\n%s", got)
	}
}

func TestRemoveIsNoopForUnknownProfileOrMissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nested", "config")
	if err := Remove(missing, "prod", "abc"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("Remove created a config file")
	}

	path := filepath.Join(t.TempDir(), "config")
	orig := "[profile mine]\nregion = eu-west-1\n"
	mustWrite(t, path, orig)
	for _, name := range []string{"mine", "absent", "default"} {
		if err := Remove(path, name, "abc"); err != nil {
			t.Fatal(err)
		}
	}
	if got := mustRead(t, path); got != orig {
		t.Fatalf("Remove touched a file it does not own:\n%s", got)
	}
}

func TestRemoveDropsRegionThatRolleAdded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	orig := "[profile prod]\noutput = json\n"
	mustWrite(t, path, orig)
	if err := Write(path, rolle); err != nil {
		t.Fatal(err)
	}
	got := mustRead(t, path)
	if !strings.Contains(got, "region = us-east-1") || !strings.Contains(got, "rolle_preexisting = true") || strings.Contains(got, prevRegion) {
		t.Fatalf("after take-over:\n%s", got)
	}
	if err := Remove(path, "prod", "abc"); err != nil {
		t.Fatal(err)
	}
	if got := mustRead(t, path); got != orig {
		t.Fatalf("section not restored without the added region:\n%s", got)
	}
}

func TestWriteRefusesEveryCredentialKey(t *testing.T) {
	for _, key := range credentialKeys {
		t.Run(key, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config")
			orig := "[profile prod]\n" + key + " = something\n"
			mustWrite(t, path, orig)
			err := Write(path, rolle)
			if err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("err = %v, want refusal naming %q", err, key)
			}
			if got := mustRead(t, path); got != orig {
				t.Fatalf("refused Write changed the file:\n%s", got)
			}
		})
	}
}

func TestWriteCreatesPrivateFileAndLeavesNoTemp(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".aws")
	path := filepath.Join(dir, "config")
	if err := Write(path, rolle); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "config" {
		t.Fatalf("dir has %d entries, want only config", len(entries))
	}
	if runtime.GOOS == "windows" {
		return
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o700 || fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("perms dir=%o file=%o, want 700/600", dirInfo.Mode().Perm(), fileInfo.Mode().Perm())
	}
}

func TestSectionNameAndQuote(t *testing.T) {
	sections := map[string]string{"default": "default", "prod": "profile prod", "my profile": "profile my profile"}
	for in, want := range sections {
		if got := sectionName(in); got != want {
			t.Errorf("sectionName(%q) = %q, want %q", in, got, want)
		}
	}
	quotes := map[string]string{"": "", "/opt/rolle": "/opt/rolle", "/Applications/Rolle.app/rolle cli": `"/Applications/Rolle.app/rolle cli"`, `C:\Program Files\rolle.exe`: `"C:\Program Files\rolle.exe"`}
	for in, want := range quotes {
		if got := quote(in); got != want {
			t.Errorf("quote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDefaultPathHonoursEnv(t *testing.T) {
	custom := filepath.Join(t.TempDir(), "aws.cfg")
	t.Setenv("AWS_CONFIG_FILE", custom)
	if got, err := DefaultPath(); err != nil || got != custom {
		t.Fatalf("DefaultPath = %q, %v; want %q", got, err, custom)
	}
	home := t.TempDir()
	t.Setenv("AWS_CONFIG_FILE", "")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if got, err := DefaultPath(); err != nil || got != filepath.Join(home, ".aws", "config") {
		t.Fatalf("DefaultPath = %q, %v", got, err)
	}
}

func TestRemoveKeepsUserRegionWhenProfileHadNone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	before := "[default]\nregion = eu-west-1\noutput = json\n"
	if err := os.WriteFile(path, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, Profile{Name: "default", SessionID: "s1", Executable: "/bin/rolle"}); err != nil {
		t.Fatal(err)
	}
	if err := Remove(path, "default", "s1"); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if !strings.Contains(string(after), "region = eu-west-1") {
		t.Fatalf("user region lost:\n%s", after)
	}
}

func TestWriteRefusesProfileShadowedByCredentialsFile(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(dir, "credentials"))
	creds := "[default]\naws_access_key_id = AKIA\naws_secret_access_key = x\n\n[other]\nregion = us-east-1\n"
	if err := os.WriteFile(filepath.Join(dir, "credentials"), []byte(creds), 0o600); err != nil {
		t.Fatal(err)
	}
	err := Write(config, Profile{Name: "default", SessionID: "s1", Executable: "/bin/rolle"})
	if err == nil || !strings.Contains(err.Error(), "shadow") {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(config); !os.IsNotExist(statErr) {
		t.Fatal("config file was written despite the shadowing keys")
	}
	// A section without keys does not shadow.
	if err := Write(config, Profile{Name: "other", SessionID: "s2", Executable: "/bin/rolle"}); err != nil {
		t.Fatal(err)
	}
}

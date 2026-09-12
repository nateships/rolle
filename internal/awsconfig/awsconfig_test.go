package awsconfig

import (
	"os"
	"path/filepath"
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

package awsconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteThenRemoveKeepsForeignProfiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte("[profile mine]\nregion = eu-west-1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := Write(path, Profile{Name: "prod", Region: "us-east-1", SessionID: "abc", Executable: "/usr/local/bin/rolle"})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	s := string(data)
	for _, want := range []string{"[profile prod]", "credential_process = /usr/local/bin/rolle creds --session abc", "rolle_session = abc", "[profile mine]"} {
		if !strings.Contains(s, want) {
			t.Fatalf("config missing %q:\n%s", want, s)
		}
	}
	if err := Remove(path, "prod"); err != nil {
		t.Fatal(err)
	}
	if err := Remove(path, "mine"); err == nil {
		t.Fatal("expected refusal to remove foreign profile")
	}
	data, _ = os.ReadFile(path)
	if strings.Contains(string(data), "prod") || !strings.Contains(string(data), "[profile mine]") {
		t.Fatalf("after remove:\n%s", data)
	}
}

func TestQuoteExecutableWithSpaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := Write(path, Profile{Name: "default", SessionID: "x", Executable: "/Applications/Rolle.app/Contents/MacOS/rolle cli"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `[default]`) || !strings.Contains(string(data), `"/Applications/Rolle.app/Contents/MacOS/rolle cli" creds`) {
		t.Fatalf("config:\n%s", data)
	}
}

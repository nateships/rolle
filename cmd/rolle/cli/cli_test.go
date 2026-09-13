package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nateships/rolle/internal/app"
	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/credcache"
	"github.com/nateships/rolle/internal/secrets"
	"github.com/nateships/rolle/internal/version"
)

// testCLI routes every command run at a throwaway workspace, AWS config, and
// in-memory secret store. It returns the service the commands use.
func testCLI(t *testing.T) *app.Service {
	t.Helper()
	dir := t.TempDir()
	s := &app.Service{
		WorkspacePath: filepath.Join(dir, "workspace.json"),
		AWSConfigPath: filepath.Join(dir, "aws", "config"),
		Executable:    "/opt/rolle",
		Secrets:       &secrets.Memory{},
		Cache:         &credcache.Cache{Dir: filepath.Join(dir, "cache")},
	}
	old := newService
	newService = func() (*app.Service, error) { return s, nil }
	t.Cleanup(func() { newService = old; svc = nil })
	return s
}

// run executes the CLI with args and returns what it printed to stdout.
func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() {
		var b bytes.Buffer
		_, _ = io.Copy(&b, r)
		done <- b.String()
	}()
	root := Root()
	root.SetOut(w)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	execErr := root.Execute()
	os.Stdout = oldStdout
	_ = w.Close()
	out := <-done
	_ = r.Close()
	return out, execErr
}

// mustRun fails the test when the command errors.
func mustRun(t *testing.T, args ...string) string {
	t.Helper()
	out, err := run(t, args...)
	if err != nil {
		t.Fatalf("rolle %s: %v", strings.Join(args, " "), err)
	}
	return out
}

// lines splits output into trimmed, non-empty lines.
func lines(out string) []string {
	var res []string
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			res = append(res, l)
		}
	}
	return res
}

// fields splits a table row on runs of spaces.
func fields(row string) []string { return strings.Fields(row) }

func TestSessionListEmpty(t *testing.T) {
	testCLI(t)
	out := mustRun(t, "session", "list")
	got := lines(out)
	if len(got) != 1 || !strings.HasPrefix(got[0], "NAME") || !strings.HasSuffix(got[0], "ID") {
		t.Fatalf("output:\n%s", out)
	}
}

func TestIntegrationAddAndList(t *testing.T) {
	s := testCLI(t)
	out := mustRun(t, "integration", "add", "aws-sso", "--alias", "acme", "--start-url", "https://acme.awsapps.com/start", "--region", "eu-west-1")
	if !strings.HasPrefix(out, "added acme (") || !strings.Contains(out, "next: rolle integration login acme") {
		t.Fatalf("add output:\n%s", out)
	}
	w, _ := s.Load()
	if len(w.Integrations) != 1 || w.Integrations[0].AWSSSO.Region != "eu-west-1" {
		t.Fatalf("workspace = %+v", w.Integrations)
	}
	out = mustRun(t, "int", "list")
	got := lines(out)
	if len(got) != 2 {
		t.Fatalf("list output:\n%s", out)
	}
	row := fields(got[1])
	if row[0] != "acme" || row[1] != "aws" || strings.Join(row[2:4], " ") != "logged out" || row[4] != w.Integrations[0].ID {
		t.Fatalf("row = %v", row)
	}
	if _, err := run(t, "integration", "add", "aws-sso", "--alias", "x"); err == nil || !strings.Contains(err.Error(), "required flag") {
		t.Fatalf("missing flags: %v", err)
	}
	mustRun(t, "integration", "remove", "acme")
	if got := lines(mustRun(t, "integration", "list")); len(got) != 1 {
		t.Fatalf("list after remove: %v", got)
	}
	if _, err := run(t, "integration", "remove", "acme"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("remove twice: %v", err)
	}
}

func TestSessionAddProfileRegionRemove(t *testing.T) {
	s := testCLI(t)
	out := mustRun(t, "session", "add", "iam-user", "--name", "dev", "--region", "us-east-1", "--access-key-id", "AKIA", "--secret-access-key", "secret")
	if !strings.HasPrefix(out, "added dev (") {
		t.Fatalf("add output:\n%s", out)
	}
	w, _ := s.Load()
	sess, err := app.FindSession(w, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Secrets.Get("aws-access-key/" + sess.ID); err != nil {
		t.Fatalf("secret not stored: %v", err)
	}
	got := lines(mustRun(t, "session", "list"))
	if len(got) != 2 {
		t.Fatalf("list: %v", got)
	}
	row := fields(got[1])
	if row[0] != "dev" || row[1] != "aws-iam-user" || row[2] != "us-east-1" || row[3] != "inactive" || row[4] != sess.ID {
		t.Fatalf("row = %v", row)
	}

	mustRun(t, "session", "profile", "dev", "work")
	if got := app.ProfileName(reload(t, s, sess.ID)); got != "work" {
		t.Fatalf("profile = %q", got)
	}
	if _, err := run(t, "session", "profile", "dev", "bad name"); err == nil {
		t.Fatal("invalid profile accepted")
	}
	mustRun(t, "session", "profile", "dev")
	if got := app.ProfileName(reload(t, s, sess.ID)); got != "default" {
		t.Fatalf("profile after reset = %q", got)
	}
	if _, err := run(t, "session", "profile"); err == nil {
		t.Fatal("profile without a session accepted")
	}

	mustRun(t, "sess", "region", sess.ID[:8], "eu-west-1")
	if got := reload(t, s, sess.ID).Region; got != "eu-west-1" {
		t.Fatalf("region = %q", got)
	}
	if _, err := run(t, "session", "region", "dev"); err == nil {
		t.Fatal("region without a value accepted")
	}

	if _, err := run(t, "session", "remove", "ghost"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("remove unknown: %v", err)
	}
	if out := mustRun(t, "session", "remove", "dev"); out != "" {
		t.Fatalf("remove printed %q", out)
	}
	if got := lines(mustRun(t, "session", "list")); len(got) != 1 {
		t.Fatalf("list after remove: %v", got)
	}
	if _, err := s.Secrets.Get("aws-access-key/" + sess.ID); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("secret not removed: %v", err)
	}
}

func reload(t *testing.T, s *app.Service, ref string) *core.Session {
	t.Helper()
	w, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	sess, err := app.FindSession(w, ref)
	if err != nil {
		t.Fatal(err)
	}
	return sess
}

func TestStartStatusStopAndEnv(t *testing.T) {
	s := testCLI(t)
	if out := mustRun(t, "status"); strings.TrimSpace(out) != "no active sessions" {
		t.Fatalf("status output:\n%s", out)
	}
	mustRun(t, "session", "add", "iam-user", "--name", "dev", "--region", "us-east-1", "--access-key-id", "AKIA", "--secret-access-key", "secret")
	if _, err := run(t, "env", "dev"); !errors.Is(err, app.ErrSessionInactive) {
		t.Fatalf("env on an inactive session: %v", err)
	}
	if _, err := run(t, "creds", "--session", "dev"); !errors.Is(err, app.ErrSessionInactive) {
		t.Fatalf("creds on an inactive session: %v", err)
	}
	out := mustRun(t, "start", "dev")
	if !strings.HasPrefix(out, "dev active until ") || !strings.Contains(out, "AWS profile: default") {
		t.Fatalf("start output:\n%s", out)
	}
	cfg, err := os.ReadFile(s.AWSConfigPath)
	if err != nil || !strings.Contains(string(cfg), "/opt/rolle creds --session ") {
		t.Fatalf("aws config: %v\n%s", err, cfg)
	}
	got := lines(mustRun(t, "status"))
	if len(got) != 2 || fields(got[1])[0] != "dev" || fields(got[1])[3] != "active" {
		t.Fatalf("status: %v", got)
	}
	out = mustRun(t, "env", "dev")
	// An IAM user has no session token. The eval output clears a stale one.
	if !strings.Contains(out, "export AWS_ACCESS_KEY_ID='AKIA'\n") || !strings.Contains(out, "export AWS_SECRET_ACCESS_KEY='secret'\n") || !strings.Contains(out, "export AWS_REGION='us-east-1'\n") || !strings.Contains(out, "unset AWS_SESSION_TOKEN\n") {
		t.Fatalf("env output:\n%s", out)
	}
	out = mustRun(t, "env", "dev", "--powershell")
	if !strings.Contains(out, "$env:AWS_ACCESS_KEY_ID = ") || !strings.Contains(out, "Remove-Item Env:AWS_SESSION_TOKEN -ErrorAction SilentlyContinue\n") {
		t.Fatalf("powershell output:\n%s", out)
	}
	out = mustRun(t, "creds", "--session", reload(t, s, "dev").ID)
	if !strings.Contains(out, `"Version":1`) || !strings.Contains(out, `"AccessKeyId":"AKIA"`) || !strings.Contains(out, `"Expiration":"`) {
		t.Fatalf("creds output:\n%s", out)
	}
	mustRun(t, "stop", "dev")
	if out := mustRun(t, "status"); strings.TrimSpace(out) != "no active sessions" {
		t.Fatalf("status after stop:\n%s", out)
	}
	if _, err := run(t, "start", "ghost"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("start unknown: %v", err)
	}
	if _, err := run(t, "start"); err == nil {
		t.Fatal("start without a session accepted")
	}
}

func TestConsolePrintForAzureSession(t *testing.T) {
	s := testCLI(t)
	w, _ := s.Load()
	w.Sessions = append(w.Sessions, core.Session{ID: "az1", Name: "sub", Kind: core.KindAzure, Status: core.StatusInactive, Azure: &core.AzureSession{SubscriptionID: "sub", TenantID: "ten-1"}})
	if err := s.Save(w); err != nil {
		t.Fatal(err)
	}
	out := mustRun(t, "console", "--print", "sub")
	if strings.TrimSpace(out) != "https://portal.azure.com/#@ten-1" {
		t.Fatalf("console output:\n%s", out)
	}
	if _, err := run(t, "console", "--print", "ghost"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("console unknown: %v", err)
	}
}

func TestSessionListShowsExpiry(t *testing.T) {
	s := testCLI(t)
	exp := time.Now().Add(90 * time.Minute).UTC()
	w, _ := s.Load()
	w.Sessions = append(w.Sessions, core.Session{ID: "az1", Name: "sub", Kind: core.KindAzure, Status: core.StatusActive, Expires: &exp, Azure: &core.AzureSession{SubscriptionID: "sub", TenantID: "ten-1"}})
	if err := s.Save(w); err != nil {
		t.Fatal(err)
	}
	if err := s.Cache.Put("az1", core.Credentials{Token: "tok", Expiration: &exp}); err != nil {
		t.Fatal(err)
	}
	got := lines(mustRun(t, "session", "list"))
	if len(got) != 2 {
		t.Fatalf("list: %v", got)
	}
	row := fields(got[1])
	if row[0] != "sub" || row[1] != "azure" || row[2] != "active" || !strings.HasPrefix(row[3], "1h") {
		t.Fatalf("row = %v", row)
	}
}

func TestResetYes(t *testing.T) {
	s := testCLI(t)
	mustRun(t, "session", "add", "iam-user", "--name", "dev", "--region", "us-east-1", "--access-key-id", "AKIA", "--secret-access-key", "secret")
	mustRun(t, "integration", "add", "aws-sso", "--alias", "acme", "--start-url", "https://acme.awsapps.com/start", "--region", "eu-west-1")
	mustRun(t, "start", "dev")
	sess := reload(t, s, "dev")
	out := mustRun(t, "reset", "--yes")
	if !strings.HasPrefix(out, "rolle reset.") {
		t.Fatalf("reset output:\n%s", out)
	}
	if _, err := os.Stat(s.WorkspacePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("workspace still present: %v", err)
	}
	if _, err := os.Stat(s.Cache.Dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cache still present: %v", err)
	}
	if _, err := s.Secrets.Get("aws-access-key/" + sess.ID); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("secret still present: %v", err)
	}
	if cfg, _ := os.ReadFile(s.AWSConfigPath); strings.Contains(string(cfg), "credential_process") {
		t.Fatalf("profile still present:\n%s", cfg)
	}
	if got := lines(mustRun(t, "session", "list")); len(got) != 1 {
		t.Fatalf("list after reset: %v", got)
	}
}

func TestVersionFlag(t *testing.T) {
	testCLI(t)
	out := mustRun(t, "--version")
	if !strings.Contains(out, "rolle version "+version.Version) {
		t.Fatalf("version output:\n%s", out)
	}
}

func TestUnknownCommand(t *testing.T) {
	testCLI(t)
	_, err := run(t, "bogus")
	if err == nil || !strings.Contains(err.Error(), `unknown command "bogus"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestServiceConstructionErrorIsReported(t *testing.T) {
	old := newService
	newService = func() (*app.Service, error) { return nil, errors.New("no keychain") }
	t.Cleanup(func() { newService = old })
	if _, err := run(t, "session", "list"); err == nil || err.Error() != "no keychain" {
		t.Fatalf("err = %v", err)
	}
}

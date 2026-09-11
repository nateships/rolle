package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nateships/rolle/internal/aws"
	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/credcache"
	"github.com/nateships/rolle/internal/secrets"
)

func testService(t *testing.T) *Service {
	dir := t.TempDir()
	return &Service{
		WorkspacePath: filepath.Join(dir, "workspace.json"),
		AWSConfigPath: filepath.Join(dir, "aws", "config"),
		Executable:    "/opt/rolle",
		Secrets:       &secrets.Memory{},
		Cache:         &credcache.Cache{Dir: filepath.Join(dir, "cache")},
	}
}

func TestIAMUserStartStopLifecycle(t *testing.T) {
	s := testService(t)
	sess, err := s.AddIAMUser(AddIAMUserInput{Name: "dev user", Region: "us-east-1", Key: aws.AccessKey{AccessKeyID: "AKIA", SecretAccessKey: "secret"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Credentials(context.Background(), "dev user"); err == nil || !strings.Contains(err.Error(), "not started") {
		t.Fatalf("expected inactive error, got %v", err)
	}
	creds, err := s.Start(context.Background(), sess.ID[:8], StartOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if creds.AccessKeyID != "AKIA" || creds.Expiration == nil {
		t.Fatalf("creds = %+v", creds)
	}
	cfg, err := os.ReadFile(s.AWSConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cfg), "[profile dev-user]") || !strings.Contains(string(cfg), "/opt/rolle creds --session "+sess.ID) {
		t.Fatalf("aws config:\n%s", cfg)
	}
	w, _ := s.Load()
	got, _ := FindSession(w, "dev user")
	if got.Status != core.StatusActive || got.Expires == nil {
		t.Fatalf("session = %+v", got)
	}
	again, err := s.Credentials(context.Background(), sess.ID)
	if err != nil || again.AccessKeyID != "AKIA" {
		t.Fatalf("credentials after start: %+v %v", again, err)
	}
	if err := s.Stop("dev user"); err != nil {
		t.Fatal(err)
	}
	cfg, _ = os.ReadFile(s.AWSConfigPath)
	if strings.Contains(string(cfg), "dev-user") {
		t.Fatalf("profile not removed:\n%s", cfg)
	}
	if err := s.RemoveSession(sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := aws.LoadAccessKey(s.Secrets, sess.ID); err != aws.ErrNoAccessKey {
		t.Fatalf("access key not deleted: %v", err)
	}
}

func TestIAMUserWithMFARequiresCode(t *testing.T) {
	s := testService(t)
	_, err := s.AddIAMUser(AddIAMUserInput{Name: "mfa", Region: "us-east-1", MFADevice: "arn:aws:iam::1:mfa/me", Key: aws.AccessKey{AccessKeyID: "A", SecretAccessKey: "B"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Start(context.Background(), "mfa", StartOptions{}); err == nil || !strings.Contains(err.Error(), "MFA code required") {
		t.Fatalf("err = %v", err)
	}
}

func TestRemoveSourceSessionRefused(t *testing.T) {
	s := testService(t)
	src, err := s.AddIAMUser(AddIAMUserInput{Name: "src", Region: "us-east-1", Key: aws.AccessKey{AccessKeyID: "A", SecretAccessKey: "B"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddAssumeRole(AddAssumeRoleInput{Name: "admin", Region: "us-east-1", RoleARN: "arn:aws:iam::1:role/admin", SourceRef: "src"}); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveSession(src.ID); err == nil {
		t.Fatal("expected refusal to remove a source session")
	}
	if err := s.RemoveSession("admin"); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveSession("src"); err != nil {
		t.Fatal(err)
	}
}

func TestFindSessionPrefixAndAmbiguity(t *testing.T) {
	w := &core.Workspace{Sessions: []core.Session{{ID: "abc123", Name: "one"}, {ID: "abd456", Name: "two"}}}
	if s, err := FindSession(w, "abc"); err != nil || s.Name != "one" {
		t.Fatalf("prefix lookup: %v %v", s, err)
	}
	if _, err := FindSession(w, "ab"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguity, got %v", err)
	}
	if _, err := FindSession(w, "zzz"); err == nil {
		t.Fatal("expected not found")
	}
}

func TestAddAWSSSOAndRemoveIntegration(t *testing.T) {
	s := testService(t)
	in, err := s.AddAWSSSO("acme", "https://acme.awsapps.com/start", "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	w, _ := s.Load()
	w.Sessions = append(w.Sessions, core.Session{ID: "s1", Name: "acct/Admin", Kind: core.KindAWSSSORole, IntegrationID: in.ID, AWS: &core.AWSSession{AccountID: "1", RoleName: "Admin"}})
	if err := s.Save(w); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveIntegration("acme"); err != nil {
		t.Fatal(err)
	}
	w, _ = s.Load()
	if len(w.Integrations) != 0 || len(w.Sessions) != 0 {
		t.Fatalf("workspace after remove = %+v", w)
	}
}

func TestSanitizeProfile(t *testing.T) {
	if got := sanitizeProfile("Acme Prod/AdministratorAccess"); got != "Acme-Prod-AdministratorAccess" {
		t.Fatalf("got %q", got)
	}
	if got := sanitizeProfile("--x--"); got != "x" {
		t.Fatalf("got %q", got)
	}
}

func TestRefreshRenewsExpiredIAMUserWithoutMFA(t *testing.T) {
	s := testService(t)
	sess, err := s.AddIAMUser(AddIAMUserInput{Name: "plain", Region: "us-east-1", Key: aws.AccessKey{AccessKeyID: "A", SecretAccessKey: "B"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Start(context.Background(), sess.ID, StartOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := s.Cache.Delete(sess.ID); err != nil {
		t.Fatal(err)
	}
	w, err := s.Refresh()
	if err != nil {
		t.Fatal(err)
	}
	got, _ := FindSession(w, sess.ID)
	if got.Status != core.StatusActive {
		t.Fatalf("status = %s, want active after renewal", got.Status)
	}
	if _, err := s.Cache.Get(sess.ID); err != nil {
		t.Fatalf("cache not repopulated: %v", err)
	}
}

func TestRefreshDeactivatesExpiredMFASession(t *testing.T) {
	s := testService(t)
	now := time.Now()
	s.Now = func() time.Time { return now }
	past := now.Add(-time.Hour)
	w, _ := s.Load()
	w.Sessions = []core.Session{{ID: "m", Name: "mfa", Kind: core.KindAWSIAMUser, Status: core.StatusActive, Expires: &past, AWS: &core.AWSSession{MFADevice: "x"}}}
	if err := s.Save(w); err != nil {
		t.Fatal(err)
	}
	w, err := s.Refresh()
	if err != nil {
		t.Fatal(err)
	}
	if w.Sessions[0].Status != core.StatusInactive {
		t.Fatalf("status = %s", w.Sessions[0].Status)
	}
}

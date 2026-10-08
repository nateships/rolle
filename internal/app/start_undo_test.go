package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nateships/rolle/internal/aws"
	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/secrets"
	"gopkg.in/ini.v1"
)

// faultStore is a secret store that can refuse writes and deletes, like a
// locked keychain.
type faultStore struct {
	secrets.Memory
	failSet, failDelete bool
}

var errLocked = errors.New("keychain locked")

func (f *faultStore) Set(key, value string) error {
	if f.failSet {
		return errLocked
	}
	return f.Memory.Set(key, value)
}

func (f *faultStore) Delete(key string) error {
	if f.failDelete {
		return errLocked
	}
	return f.Memory.Delete(key)
}

// faultService is testService with a credential cache that can fail.
func faultService(t *testing.T) (*Service, *faultStore) {
	t.Helper()
	s := testService(t)
	store := &faultStore{}
	s.Cache.Store = store
	return s, store
}

// profileOwner returns the session ID that the AWS config file names for
// credential_process, or "" when no section names one.
func profileOwner(t *testing.T, s *Service) string {
	t.Helper()
	cfg, err := os.ReadFile(s.AWSConfigPath)
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(cfg), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok && strings.TrimSpace(k) == "rolle_session" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// readOnlyWorkspace makes the next workspace save fail.
func readOnlyWorkspace(t *testing.T, s *Service) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions do not block writes on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	// The AWS config directory is below the workspace directory. Create it
	// first, so that profile writes still work.
	if err := os.MkdirAll(filepath.Dir(s.AWSConfigPath), 0o700); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(s.WorkspacePath)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
}

// assertOwner checks that the profile names owner and that owner still
// answers credential_process.
func assertOwner(t *testing.T, s *Service, owner core.Session) {
	t.Helper()
	if got := profileOwner(t, s); got != owner.ID {
		t.Fatalf("profile names %q, want %s", got, owner.ID)
	}
	if _, err := s.Credentials(context.Background(), owner.ID); err != nil {
		t.Fatalf("credentials of %s after a failed start: %v", owner.Name, err)
	}
}

func TestFailedTakeOverKeepsProfileOwner(t *testing.T) {
	s, store := faultService(t)
	a := addIAMUser(t, s, "a")
	addIAMUser(t, s, "b")
	if _, err := s.Start(context.Background(), "a", StartOptions{}); err != nil {
		t.Fatal(err)
	}
	store.failDelete = true
	if _, err := s.Start(context.Background(), "b", StartOptions{}); !errors.Is(err, errLocked) {
		t.Fatalf("start b = %v", err)
	}
	store.failDelete = false
	assertOwner(t, s, a)
}

func TestFailedSaveKeepsProfileOwner(t *testing.T) {
	s := testService(t)
	a := addIAMUser(t, s, "a")
	addIAMUser(t, s, "b")
	if _, err := s.Start(context.Background(), "a", StartOptions{}); err != nil {
		t.Fatal(err)
	}
	readOnlyWorkspace(t, s)
	if _, err := s.Start(context.Background(), "b", StartOptions{}); err == nil {
		t.Fatal("start b succeeded with a read-only workspace")
	}
	assertOwner(t, s, a)
}

func TestFailedSaveWithoutOwnerRemovesProfile(t *testing.T) {
	s := testService(t)
	addIAMUser(t, s, "b")
	readOnlyWorkspace(t, s)
	if _, err := s.Start(context.Background(), "b", StartOptions{}); err == nil {
		t.Fatal("start b succeeded with a read-only workspace")
	}
	if got := profileOwner(t, s); got != "" {
		t.Fatalf("profile names %q after a failed start", got)
	}
}

func TestFailedRestartKeepsOwnProfile(t *testing.T) {
	s := testService(t)
	a := addIAMUser(t, s, "a")
	if _, err := s.Start(context.Background(), "a", StartOptions{}); err != nil {
		t.Fatal(err)
	}
	readOnlyWorkspace(t, s)
	if _, err := s.Start(context.Background(), "a", StartOptions{}); err == nil {
		t.Fatal("restart succeeded with a read-only workspace")
	}
	assertOwner(t, s, a)
}

// stubSTS answers every STS call with temporary credentials.
func stubSTS(t *testing.T) {
	t.Helper()
	missing := filepath.Join(t.TempDir(), "absent")
	t.Setenv("AWS_CONFIG_FILE", missing)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", missing)
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_MAX_ATTEMPTS", "1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		action := r.PostForm.Get("Action")
		w.Header().Set("Content-Type", "text/xml")
		fmt.Fprintf(w, `<%sResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><%sResult><Credentials><AccessKeyId>ASIATEST</AccessKeyId><SecretAccessKey>sts-secret</SecretAccessKey><SessionToken>sts-token</SessionToken><Expiration>2099-01-01T00:00:00Z</Expiration></Credentials></%sResult><ResponseMetadata><RequestId>r1</RequestId></ResponseMetadata></%sResponse>`, action, action, action, action)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("AWS_ENDPOINT_URL_STS", srv.URL)
}

func TestRefusedProfileDropsCachedCredentials(t *testing.T) {
	stubSTS(t)
	s := testService(t)
	mfa, err := s.AddIAMUser(AddIAMUserInput{Name: "mfa", Region: "us-east-1", MFADevice: "arn:aws:iam::1:mfa/me", Key: aws.AccessKey{AccessKeyID: "AKIAMFA", SecretAccessKey: "secret"}})
	if err != nil {
		t.Fatal(err)
	}
	// Another tool owns the default profile, so rolle refuses to write it.
	if err := os.MkdirAll(filepath.Dir(s.AWSConfigPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.AWSConfigPath, []byte("[default]\naws_access_key_id = AKIAOTHER\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Start(context.Background(), "mfa", StartOptions{MFACode: "123456"}); err == nil || !strings.Contains(err.Error(), "another tool") {
		t.Fatalf("start mfa = %v", err)
	}
	if _, err := s.Cache.Get(mfa.ID); err == nil {
		t.Fatal("credentials stay cached for a session that did not start")
	}
}

func TestFailedCachePutKeepsProfileOwner(t *testing.T) {
	stubSTS(t)
	s, store := faultService(t)
	a := addIAMUser(t, s, "a")
	if _, err := s.AddIAMUser(AddIAMUserInput{Name: "mfa", Region: "us-east-1", MFADevice: "arn:aws:iam::1:mfa/me", Key: aws.AccessKey{AccessKeyID: "AKIAMFA", SecretAccessKey: "secret"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Start(context.Background(), "a", StartOptions{}); err != nil {
		t.Fatal(err)
	}
	store.failSet = true
	if _, err := s.Start(context.Background(), "mfa", StartOptions{MFACode: "123456"}); !errors.Is(err, errLocked) {
		t.Fatalf("start mfa = %v", err)
	}
	assertOwner(t, s, a)
}

// sectionOwner returns the session ID that the section of profile names, or
// "" when the section is absent or names no session.
func sectionOwner(t *testing.T, s *Service, profile string) string {
	t.Helper()
	f, err := ini.Load(s.AWSConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	name := "profile " + profile
	if profile == "default" {
		name = profile
	}
	sec, err := f.GetSection(name)
	if err != nil {
		return ""
	}
	return sec.Key("rolle_session").String()
}

// assertActiveOn checks that the workspace keeps the session active on
// profile, that the profile names the session, and that the session still
// answers credential_process.
func assertActiveOn(t *testing.T, s *Service, id, profile string) {
	t.Helper()
	sess := session(t, s, id)
	if sess.Status != core.StatusActive || ProfileName(sess) != profile {
		t.Fatalf("%s is %s on profile %q, want active on %q", sess.Name, sess.Status, ProfileName(sess), profile)
	}
	if got := sectionOwner(t, s, profile); got != id {
		t.Fatalf("profile %q names %q, want %s", profile, got, id)
	}
	if _, err := s.Credentials(context.Background(), id); err != nil {
		t.Fatalf("credentials of %s after a failed profile change: %v", sess.Name, err)
	}
}

// startOnTwoProfiles starts a on profile work and b on the default profile.
func startOnTwoProfiles(t *testing.T, s *Service) (a, b core.Session) {
	t.Helper()
	a = addIAMUser(t, s, "a")
	b = addIAMUser(t, s, "b")
	if err := s.SetProfile(a.ID, "work"); err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"a", "b"} {
		if _, err := s.Start(context.Background(), ref, StartOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	return a, b
}

func TestFailedProfileTakeOverRestoresProfiles(t *testing.T) {
	s, store := faultService(t)
	a, b := startOnTwoProfiles(t, s)
	store.failDelete = true
	if err := s.SetProfile(b.ID, "work"); !errors.Is(err, errLocked) {
		t.Fatalf("set profile of b = %v", err)
	}
	store.failDelete = false
	assertActiveOn(t, s, a.ID, "work")
	assertActiveOn(t, s, b.ID, "default")
}

func TestFailedProfileSaveRestoresProfiles(t *testing.T) {
	s := testService(t)
	a, b := startOnTwoProfiles(t, s)
	readOnlyWorkspace(t, s)
	if err := s.SetProfile(b.ID, "work"); err == nil {
		t.Fatal("profile change succeeded with a read-only workspace")
	}
	assertActiveOn(t, s, a.ID, "work")
	assertActiveOn(t, s, b.ID, "default")
}

func TestFailedProfileSaveWithoutOwnerRemovesSection(t *testing.T) {
	s := testService(t)
	b := addIAMUser(t, s, "b")
	if _, err := s.Start(context.Background(), "b", StartOptions{}); err != nil {
		t.Fatal(err)
	}
	readOnlyWorkspace(t, s)
	if err := s.SetProfile(b.ID, "work"); err == nil {
		t.Fatal("profile change succeeded with a read-only workspace")
	}
	if cfg := awsConfig(t, s); strings.Contains(cfg, "[profile work]") {
		t.Fatalf("config after a failed profile change:\n%s", cfg)
	}
	assertActiveOn(t, s, b.ID, "default")
}

func TestSetProfileRefusesSourceProfile(t *testing.T) {
	stubSTS(t)
	s := testService(t)
	src := addIAMUser(t, s, "src")
	role, err := s.AddAssumeRole(AddAssumeRoleInput{Name: "admin", Region: "us-east-1", RoleARN: "arn:aws:iam::1:role/admin", SourceRef: "src", Profile: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"src", "admin"} {
		if _, err := s.Start(context.Background(), ref, StartOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SetProfile(role.ID, ""); err == nil || !strings.Contains(err.Error(), `both write AWS profile "default"`) {
		t.Fatalf("SetProfile(admin) = %v, want a shared-profile refusal", err)
	}
	assertActiveOn(t, s, src.ID, "default")
	assertActiveOn(t, s, role.ID, "admin")
}

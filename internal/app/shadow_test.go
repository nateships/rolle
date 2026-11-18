package app

import (
	"context"
	"errors"
	"github.com/nateships/rolle/internal/awsconfig"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nateships/rolle/internal/aws"
	"github.com/nateships/rolle/internal/core"
)

func TestShadowedProfilesNamesTheFileWithStaticKeys(t *testing.T) {
	home := fakeHome(t)
	s := testService(t)
	credPath := filepath.Join(home, ".aws", "credentials")
	// Display shortens the path with the OS separator, so Windows shows backslashes.
	shown := awsconfig.Display(credPath)
	writeFile(t, credPath, "[default]\naws_access_key_id = AKIA1\naws_secret_access_key = s1\n\n[work]\naws_access_key_id = AKIA2\naws_secret_access_key = s2\n")
	// A profile another tool configures in the config file is a conflict
	// too, but RemoveForeignKeys clears that one, not RemoveStaticProfile.
	writeFile(t, s.AWSConfigPath, "[profile foreign]\ncredential_process = other-tool\n")

	if _, err := s.AddIAMUser(AddIAMUserInput{Name: "dev", Region: "us-east-1", Key: aws.AccessKey{AccessKeyID: "A", SecretAccessKey: "B"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddIAMUser(AddIAMUserInput{Name: "work", Region: "us-east-1", Profile: "work", Key: aws.AccessKey{AccessKeyID: "C", SecretAccessKey: "D"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddIAMUser(AddIAMUserInput{Name: "free", Region: "us-east-1", Profile: "free", Key: aws.AccessKey{AccessKeyID: "E", SecretAccessKey: "F"}}); err != nil {
		t.Fatal(err)
	}
	w, _ := s.Load()
	w.Sessions = append(w.Sessions, core.Session{ID: "az1", Name: "default", Kind: core.KindAzure, Azure: &core.AzureSession{SubscriptionID: "sub"}})
	if err := s.Save(w); err != nil {
		t.Fatal(err)
	}

	// Only AWS profiles with static keys of the same name are in the map,
	// with the file in display form.
	got := s.ShadowedProfiles(w)
	if len(got) != 2 || got["default"] != shown || got["work"] != shown {
		t.Fatalf("shadowed = %v", got)
	}
	if sh := s.ProfileShadow("default"); sh == nil || !sh.Static || sh.Path != credPath {
		t.Fatalf("ProfileShadow(default) = %+v", sh)
	}
	if sh := s.ProfileShadow("foreign"); sh == nil || sh.Static || sh.Path != s.AWSConfigPath {
		t.Fatalf("ProfileShadow(foreign) = %+v", sh)
	}
	if sh := s.ProfileShadow("free"); sh != nil {
		t.Fatalf("ProfileShadow(free) = %+v", sh)
	}
}

func TestRemoveStaticProfileClearsOneSectionAndNotifies(t *testing.T) {
	home := fakeHome(t)
	s := testService(t)
	credPath := filepath.Join(home, ".aws", "credentials")
	// Display shortens the path with the OS separator, so Windows shows backslashes.
	shown := awsconfig.Display(credPath)
	writeFile(t, credPath, "[default]\naws_access_key_id = AKIA1\naws_secret_access_key = s1\nregion = us-east-1\n\n[work]\naws_access_key_id = AKIA2\naws_secret_access_key = s2\n")
	notified := 0
	s.OnChange = func() { notified++ }

	if err := s.RemoveStaticProfile(""); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("empty name = %v", err)
	}
	if err := s.RemoveStaticProfile("default"); err != nil {
		t.Fatal(err)
	}
	// The whole section goes: a region left behind would win over the
	// config file. The other section stays.
	data, _ := os.ReadFile(credPath)
	if strings.Contains(string(data), "[default]") || strings.Contains(string(data), "us-east-1") || !strings.Contains(string(data), "AKIA2") {
		t.Fatalf("credentials after remove:\n%s", data)
	}
	if notified != 1 {
		t.Fatalf("OnChange ran %d times, want 1", notified)
	}
	if sh := s.ProfileShadow("default"); sh != nil {
		t.Fatalf("still shadowed: %+v", sh)
	}
	st := s.StaticProfiles()
	if len(st.Profiles) != 1 || st.Profiles[0].Name != "work" || st.Path != shown {
		t.Fatalf("static profiles = %+v", st)
	}
	// A section that is already gone is not an error; the window still
	// refreshes.
	if err := s.RemoveStaticProfile("default"); err != nil || notified != 2 {
		t.Fatalf("second remove = %v, notified %d", err, notified)
	}
	if err := s.RemoveStaticProfile("work"); err != nil {
		t.Fatal(err)
	}
	if st := s.StaticProfiles(); len(st.Profiles) != 0 || st.Path != shown {
		t.Fatalf("static profiles after clearing = %+v", st)
	}
}

func TestRemoveForeignKeysLetsARefusedSessionStart(t *testing.T) {
	stubSTS(t)
	s := testService(t)
	sess := addIAMUser(t, s, "a")
	shown := awsconfig.Display(s.AWSConfigPath)
	writeFile(t, s.AWSConfigPath, "[default]\nsso_session = acme\nsso_account_id = 111122223333\nsso_role_name = Admin\noutput = json\n\n[sso-session acme]\nsso_start_url = https://acme.awsapps.com/start\nsso_region = us-east-1\n")
	notified := 0
	s.OnChange = func() { notified++ }

	if _, err := s.Start(context.Background(), "a", StartOptions{}); !errors.Is(err, awsconfig.ErrForeign) {
		t.Fatalf("start = %v, want ErrForeign", err)
	}
	// The listing shows the file and the lines that go.
	fp := s.ForeignProfile("default")
	if fp.Path != shown || len(fp.Keys) != 3 || fp.Keys[0] != (awsconfig.ForeignKey{Name: "sso_session", Value: "acme"}) {
		t.Fatalf("ForeignProfile(default) = %+v", fp)
	}
	if fp := s.ForeignProfile("free"); fp.Path != shown || fp.Keys == nil || len(fp.Keys) != 0 {
		t.Fatalf("ForeignProfile(free) = %+v", fp)
	}

	if err := s.RemoveForeignKeys(""); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("empty name = %v", err)
	}
	notified = 0
	if err := s.RemoveForeignKeys("default"); err != nil {
		t.Fatal(err)
	}
	if notified != 1 {
		t.Fatalf("OnChange ran %d times, want 1", notified)
	}
	if sh := s.ProfileShadow("default"); sh != nil {
		t.Fatalf("still in conflict: %+v", sh)
	}

	// The session starts onto the profile. The other keys of the section and
	// the sso-session block of the other tool stay.
	if _, err := s.Start(context.Background(), "a", StartOptions{}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(s.AWSConfigPath)
	got := string(data)
	for _, want := range []string{"rolle_session = " + sess.ID, "output = json", "[sso-session acme]", "sso_start_url = https://acme.awsapps.com/start"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
	for _, gone := range []string{"sso_session", "sso_account_id", "sso_role_name"} {
		if strings.Contains(got, gone) {
			t.Errorf("%s still present:\n%s", gone, got)
		}
	}
}

func TestFindTagMatchesCaseInsensitively(t *testing.T) {
	s := testService(t)
	if err := s.AddTag(core.Tag{Name: "Prod"}); err != nil {
		t.Fatal(err)
	}
	w, _ := s.Load()
	tag, err := FindTag(w, "prod")
	if err != nil || tag.Name != "Prod" {
		t.Fatalf("FindTag = %+v, %v", tag, err)
	}
	if _, err := FindTag(w, "staging"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("FindTag unknown = %v", err)
	}
}

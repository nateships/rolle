package app

import (
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
	// too, but not one rolle can clear.
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
	if sh := s.ProfileShadow("default"); sh == nil || !sh.Fixable || sh.Path != credPath {
		t.Fatalf("ProfileShadow(default) = %+v", sh)
	}
	if sh := s.ProfileShadow("foreign"); sh == nil || sh.Fixable || sh.Path != s.AWSConfigPath {
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

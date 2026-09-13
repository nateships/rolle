package main

import (
	"github.com/nateships/rolle/internal/awsconfig"
	"os"
	"path/filepath"
	"testing"
)

// writeCredentials puts a shared credentials file with static keys in the
// fake home and routes the AWS SDK path at it. It returns the path in the
// display form the app shows, which uses the OS separator.
func writeCredentials(t *testing.T, body string) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	credPath := filepath.Join(home, ".aws", "credentials")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credPath)
	if err := os.MkdirAll(filepath.Dir(credPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(credPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return awsconfig.Display(credPath)
}

func TestWorkspaceMarksShadowedProfiles(t *testing.T) {
	r := testrolle(t)
	shown := writeCredentials(t, "[default]\naws_access_key_id = AKIA1\naws_secret_access_key = s1\n\n[personal]\naws_access_key_id = AKIA2\naws_secret_access_key = s2\n")
	// The first IAM user owns the "default" profile, which the static keys
	// in the credentials file shadow.
	addIAMUser(t, r, "dev")
	w, err := r.Workspace()
	if err != nil {
		t.Fatal(err)
	}
	if len(w.ShadowedProfiles) != 1 || w.ShadowedProfiles["default"] != shown {
		t.Fatalf("shadowed = %v", w.ShadowedProfiles)
	}
	// ProfileShadow answers for any profile name, in display form, and
	// only for a conflict the Remove keys dialog can clear.
	if got := r.ProfileShadow("personal"); got != shown {
		t.Fatalf("ProfileShadow(personal) = %q", got)
	}
	if got := r.ProfileShadow("free"); got != "" {
		t.Fatalf("ProfileShadow(free) = %q", got)
	}
	if err := os.MkdirAll(filepath.Dir(r.svc.AWSConfigPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.svc.AWSConfigPath, []byte("[profile foreign]\ncredential_process = other-tool\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := r.ProfileShadow("foreign"); got != "" {
		t.Fatalf("ProfileShadow(foreign) = %q, want nothing for a config file conflict", got)
	}

	st := r.StaticProfiles()
	if st.Path != shown || len(st.Profiles) != 2 || st.Profiles[0].Imported || st.Profiles[1].Imported {
		t.Fatalf("static profiles = %+v", st)
	}
	// Importing a profile makes a session with its name and marks the key.
	sess, err := r.ImportIAMUser("personal")
	if err != nil {
		t.Fatal(err)
	}
	if sess.Name != "personal" || sess.AWS.Profile != "personal" || sess.AWS.AccessKeyID != "AKIA2" {
		t.Fatalf("imported = %+v aws = %+v", sess, sess.AWS)
	}
	if st := r.StaticProfiles(); st.Profiles[0].Imported || !st.Profiles[1].Imported {
		t.Fatalf("static profiles after import = %+v", st.Profiles)
	}
	if w, _ = r.Workspace(); len(w.ShadowedProfiles) != 2 || w.ShadowedProfiles["personal"] != shown {
		t.Fatalf("shadowed after import = %v", w.ShadowedProfiles)
	}
	// Removing the static keys frees the profile for rolle.
	if err := r.RemoveStaticProfile("personal"); err != nil {
		t.Fatal(err)
	}
	if err := r.RemoveStaticProfile(""); err == nil {
		t.Fatal("empty profile name accepted")
	}
	if w, _ = r.Workspace(); len(w.ShadowedProfiles) != 1 || w.ShadowedProfiles["default"] == "" {
		t.Fatalf("shadowed after remove = %v", w.ShadowedProfiles)
	}
	if st := r.StaticProfiles(); len(st.Profiles) != 1 || st.Profiles[0].Name != "default" {
		t.Fatalf("static profiles after remove = %+v", st.Profiles)
	}
	if _, err := r.ImportIAMUser("personal"); err == nil {
		t.Fatal("import of removed keys accepted")
	}
}

package main

import (
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nateships/rolle/internal/version"
)

// withManaged makes updatesManaged report on and a release version for one test.
func withManaged(t *testing.T, on bool) {
	t.Helper()
	prevManaged, prevVersion := updatesManaged, version.Version
	t.Cleanup(func() { updatesManaged, version.Version = prevManaged, prevVersion })
	updatesManaged = func() bool { return on }
	version.Version = "1.2.3"
	// On Linux an install without APPIMAGE is a package manager's, and that
	// check runs first. An AppImage path lets the managed check run.
	t.Setenv("APPIMAGE", "/tmp/rolle.AppImage")
}

func TestCheckForUpdatesReportsManaged(t *testing.T) {
	withManaged(t, true)
	r := testrolle(t)
	// The updater of this app is not configured: reaching it would fail.
	r.app = &application.App{}
	info, err := r.CheckForUpdates()
	if err != nil {
		t.Fatal(err)
	}
	if info.Enabled || info.State != stateManaged {
		t.Fatalf("info = %+v", info)
	}
}

func TestInstallUpdateRefusesManaged(t *testing.T) {
	withManaged(t, true)
	r := testrolle(t)
	r.app = &application.App{}
	if err := r.InstallUpdate(); err == nil || !strings.Contains(err.Error(), "organization") {
		t.Fatalf("err = %v", err)
	}
}

func TestUpdateBlock(t *testing.T) {
	withManaged(t, false)
	if state, err := updateBlock(); state != "" || err != nil {
		t.Fatalf("release, not managed: %q, %v", state, err)
	}
	updatesManaged = func() bool { return true }
	if state, err := updateBlock(); state != stateManaged || err == nil {
		t.Fatalf("managed: %q, %v", state, err)
	}
	// A dev build wins over the profile, and InstallUpdate stays quiet there.
	version.Version = "0.0.1-dev"
	if state, err := updateBlock(); state != "disabled" || err != nil {
		t.Fatalf("dev build: %q, %v", state, err)
	}
}

func TestBackgroundCheckSkipsManaged(t *testing.T) {
	withManaged(t, true)
	// No app or service is attached: reaching either would panic.
	backgroundCheck(nil, nil)
}

func TestUpdatesManagedBinding(t *testing.T) {
	r := testrolle(t)
	for _, on := range []bool{true, false} {
		withManaged(t, on)
		if got := r.UpdatesManaged(); got != on {
			t.Errorf("UpdatesManaged() = %v, want %v", got, on)
		}
	}
}

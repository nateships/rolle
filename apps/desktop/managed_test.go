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

func TestSetupUpdaterSkipsManaged(t *testing.T) {
	withManaged(t, true)
	// No app is attached: reaching the updater would panic.
	if err := setupUpdater(nil, nil); err != nil {
		t.Fatal(err)
	}
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

// No test machine carries a rolle configuration profile.
func TestManagedByProfileOffWithoutProfile(t *testing.T) {
	if managedByProfile() {
		t.Fatal("managedByProfile() = true without a profile")
	}
}

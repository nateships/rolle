package main

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/nateships/rolle/internal/core"
)

func TestModTimeIsZeroForMissingFile(t *testing.T) {
	dir := t.TempDir()
	if !modTime(filepath.Join(dir, "missing")).IsZero() {
		t.Fatal("missing file has a modification time")
	}
	p := filepath.Join(dir, "present")
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if modTime(p).IsZero() {
		t.Fatal("present file has no modification time")
	}
}

func TestCurrentSettingsFallsBackToDefaults(t *testing.T) {
	r := testrolle(t)
	if got := currentSettings(r.svc); !reflect.DeepEqual(got, core.DefaultSettings()) {
		t.Fatalf("settings = %+v", got)
	}
	st, _ := r.Settings()
	st.Theme = "dark"
	if _, err := r.UpdateSettings(st); err != nil {
		t.Fatal(err)
	}
	if got := currentSettings(r.svc); got.Theme != "dark" {
		t.Fatalf("settings = %+v", got)
	}
	if err := os.WriteFile(r.svc.WorkspacePath, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := currentSettings(r.svc); !reflect.DeepEqual(got, core.DefaultSettings()) {
		t.Fatalf("settings from corrupt workspace = %+v", got)
	}
}

func TestSettingsOfFallsBackToDefaults(t *testing.T) {
	if got := settingsOf(nil); !reflect.DeepEqual(got, core.DefaultSettings()) {
		t.Fatalf("nil workspace = %+v", got)
	}
	w := &core.Workspace{Settings: &core.Settings{NotifyLeadMinutes: 5}}
	if got := settingsOf(w); got.NotifyLeadMinutes != 5 || got.AssumeRoleMinutes != 60 {
		t.Fatalf("stored settings not normalized: %+v", got)
	}
}

func TestUntilNextCrossingPicksTheNearestBoundary(t *testing.T) {
	now := time.Now()
	at := func(d time.Duration) *time.Time { x := now.Add(d); return &x }
	st := core.DefaultSettings() // two minute lead
	if got := untilNextCrossing(nil, st, now); got != time.Hour {
		t.Fatalf("nil workspace waits %v", got)
	}
	w := &core.Workspace{
		Integrations: []core.Integration{
			{ID: "i1", AWSSSO: &core.AWSSSOIntegration{TokenExpires: at(portalWarnBefore + 10*time.Minute)}},
		},
		Sessions: []core.Session{
			{ID: "a", IntegrationID: "i1", Status: core.StatusActive, Expires: at(5 * time.Minute)},
			{ID: "b", Status: core.StatusInactive, Expires: at(time.Minute)},
		},
	}
	// The session enters its lead at three minutes, before the portal at ten.
	if got := untilNextCrossing(w, st, now); got != 3*time.Minute+time.Second {
		t.Fatalf("next crossing in %v", got)
	}
	// Inside the lead, the next boundary is the expiry itself.
	if got := untilNextCrossing(w, st, now.Add(4*time.Minute)); got != time.Minute+time.Second {
		t.Fatalf("expiry in %v", got)
	}
	// A longer lead moves the crossing earlier, here to right now.
	st.NotifyLeadMinutes = 10
	if got := untilNextCrossing(w, st, now); got != 5*time.Minute+time.Second {
		t.Fatalf("with a ten minute lead the expiry is next, got %v", got)
	}
	// Once the session is gone, only the portal remains.
	w.Sessions[0].Status = core.StatusInactive
	if got := untilNextCrossing(w, st, now); got != time.Hour {
		t.Fatalf("portal without active sessions waits %v", got)
	}
}

func TestInBundleMatchesPlatform(t *testing.T) {
	// The test binary is not an app bundle; other platforms always qualify.
	if got := inBundle(); got != (runtime.GOOS != "darwin") {
		t.Fatalf("inBundle = %v on %s", got, runtime.GOOS)
	}
}

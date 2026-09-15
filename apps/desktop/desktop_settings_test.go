package main

import (
	"errors"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nateships/rolle/internal/core"
)

func TestDockPolicy(t *testing.T) {
	if got := dockPolicy(false); got != application.ActivationPolicyRegular {
		t.Fatalf("shown dock = %v", got)
	}
	if got := dockPolicy(true); got != application.ActivationPolicyAccessory {
		t.Fatalf("hidden dock = %v", got)
	}
}

// The OS-side settings apply before the save, so a failure keeps the stored
// settings and the hook sees the normalized new values.
func TestUpdateSettingsAppliesBeforeSave(t *testing.T) {
	r := testrolle(t)
	var gotPrev, gotNext core.Settings
	fail := errors.New("no login item here")
	r.applySettings = func(prev, next core.Settings) error {
		gotPrev, gotNext = prev, next
		return fail
	}
	in := core.DefaultSettings()
	in.LoginItem, in.HideDock, in.AssumeRoleMinutes = true, true, 1
	if _, err := r.UpdateSettings(in); !errors.Is(err, fail) {
		t.Fatalf("err = %v, want the apply error", err)
	}
	if gotPrev.LoginItem || gotPrev.HideDock || !gotNext.LoginItem || !gotNext.HideDock {
		t.Fatalf("hook saw prev=%+v next=%+v", gotPrev, gotNext)
	}
	if gotNext.AssumeRoleMinutes != 60 {
		t.Fatalf("hook saw unnormalized settings: %+v", gotNext)
	}
	if st, _ := r.Settings(); st.LoginItem || st.HideDock {
		t.Fatalf("a failed apply saved the settings: %+v", st)
	}

	r.applySettings = func(core.Settings, core.Settings) error { return nil }
	if _, err := r.UpdateSettings(in); err != nil {
		t.Fatal(err)
	}
	if st, _ := r.Settings(); !st.LoginItem || !st.HideDock {
		t.Fatalf("settings after apply = %+v", st)
	}
}

// A save that fails after the apply runs the hook again the other way, so
// the OS matches the settings the store kept.
func TestUpdateSettingsUndoesApplyWhenSaveFails(t *testing.T) {
	r := testrolle(t)
	var calls [][2]core.Settings
	r.applySettings = func(prev, next core.Settings) error {
		calls = append(calls, [2]core.Settings{prev, next})
		return nil
	}
	in := core.DefaultSettings()
	in.LoginItem = true
	in.ProxyURL = "::not a url" // The network config rejects it, so the save fails.
	if _, err := r.UpdateSettings(in); err == nil {
		t.Fatal("a bad proxy URL saved")
	}
	if len(calls) != 2 {
		t.Fatalf("apply ran %d times, want 2 (apply, undo)", len(calls))
	}
	if !calls[0][1].LoginItem || !calls[1][0].LoginItem || calls[1][1].LoginItem {
		t.Fatalf("apply saw %+v, undo saw %+v", calls[0], calls[1])
	}
	if st, _ := r.Settings(); st.LoginItem {
		t.Fatalf("a failed save stored the settings: %+v", st)
	}
}

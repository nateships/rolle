package main

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

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

func TestInBundleMatchesPlatform(t *testing.T) {
	// The test binary is not an app bundle; other platforms always qualify.
	if got := inBundle(); got != (runtime.GOOS != "darwin") {
		t.Fatalf("inBundle = %v on %s", got, runtime.GOOS)
	}
}

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// fakeBundle writes an app bundle with a helper under dir and returns the
// executable path inside it.
func fakeBundle(t *testing.T, dir string) string {
	t.Helper()
	exe := filepath.Join(dir, "rolle.app", "Contents", "MacOS", "rolle")
	helper := filepath.Join(dir, "rolle.app", "Contents", "Helpers", "rolle")
	for _, p := range []string{exe, helper} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return exe
}

func TestDevBuildSimulatesCLIInstall(t *testing.T) {
	if !devMode {
		t.Skip("production build")
	}
	// The test binary is not in a bundle, so the dev simulation is on.
	if !devSimulated() {
		t.Fatal("dev simulation off outside a bundle")
	}
	old := devCLIInstalled
	devCLIInstalled = false
	t.Cleanup(func() { devCLIInstalled = old })

	r := &RolleService{}
	if st := r.CLIStatus(); st.Installed || st.Target == "" || st.Reason != "" {
		t.Fatalf("before install = %+v", st)
	}
	st, err := r.InstallCLI()
	if err != nil {
		t.Fatal(err)
	}
	if !st.Installed || st.Path != cliLink {
		t.Fatalf("after install = %+v", st)
	}
	if err := r.UninstallCLI(); err != nil {
		t.Fatal(err)
	}
	if st := r.CLIStatus(); st.Installed {
		t.Fatalf("after uninstall = %+v", st)
	}
}

func TestCLIStatusOutsideABundleIsUnsupported(t *testing.T) {
	if st := cliStatus("/usr/local/bin/rolle-desktop"); st.Reason != "unsupported" {
		t.Fatalf("status = %+v", st)
	}
	// A bundle without the helper is a dev bundle.
	dir := t.TempDir()
	exe := filepath.Join(dir, "rolle.app", "Contents", "MacOS", "rolle")
	if st := cliStatus(exe); st.Reason != "unsupported" {
		t.Fatalf("status = %+v", st)
	}
}

func TestCLIStatusFindsHelperInBundle(t *testing.T) {
	if runtime.GOOS != "darwin" {
		if st := cliStatus(fakeBundle(t, t.TempDir())); st.Reason != "unsupported" {
			t.Fatalf("status off macOS = %+v", st)
		}
		return
	}
	// An empty PATH keeps a rolle on this machine out of the result.
	t.Setenv("PATH", t.TempDir())
	dir := t.TempDir()
	st := cliStatus(fakeBundle(t, dir))
	if st.Reason != "" || st.Target != filepath.Join(dir, "rolle.app", "Contents", "Helpers", "rolle") {
		t.Fatalf("status = %+v", st)
	}
	_, linkErr := os.Lstat(cliLink)
	if st.Installed != (linkErr == nil) {
		t.Fatalf("installed = %v, link present = %v", st.Installed, linkErr == nil)
	}
	if st.Installed && st.Path != cliLink {
		t.Fatalf("path = %q", st.Path)
	}

	moved := cliStatus(fakeBundle(t, filepath.Join(dir, "Downloads")))
	if moved.Reason != "move" {
		t.Fatalf("status from Downloads = %+v", moved)
	}
}

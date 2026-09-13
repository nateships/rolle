package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestUpdateTargetForABundleNeedsElevationWhenTheFolderIsLocked(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bundle paths and mode bits")
	}
	dir := t.TempDir()
	exe := fakeBundle(t, dir)
	bundle := filepath.Join(dir, "rolle.app")
	if target, elevate := updateTarget("darwin", exe, ""); target != bundle || elevate {
		t.Fatalf("writable bundle = %q %v", target, elevate)
	}
	if os.Getuid() == 0 {
		t.Skip("root writes anywhere")
	}
	// A standard account cannot replace a bundle in /Applications: the
	// folder refuses. The swap then runs through the administrator prompt.
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if target, elevate := updateTarget("darwin", exe, ""); target != bundle || !elevate {
		t.Fatalf("bundle in a locked folder = %q %v", target, elevate)
	}
	// On Windows the executable's folder decides, such as Program Files.
	if target, elevate := updateTarget("windows", filepath.Join(dir, "rolle.exe"), ""); target != filepath.Join(dir, "rolle.exe") || !elevate {
		t.Fatalf("executable in a locked folder = %q %v", target, elevate)
	}
}

func TestReplaceFileLeavesNothingWhenTheCopyFails(t *testing.T) {
	dir := t.TempDir()
	staged := filepath.Join(dir, "staged.AppImage")
	if err := os.WriteFile(staged, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "missing", "rolle.AppImage")
	if err := replaceFile(staged, target); err == nil {
		t.Fatal("copy into a missing directory accepted")
	}
	if _, err := os.Stat(target + ".new"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary copy left behind: %v", err)
	}
}

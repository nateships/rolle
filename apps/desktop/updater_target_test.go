package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestUpdateTargetForABundleInALockedFolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bundle paths and mode bits")
	}
	dir := t.TempDir()
	exe := fakeBundle(t, dir)
	bundle := filepath.Join(dir, "rolle.app")
	if target, swap := updateTarget("darwin", exe, ""); target != bundle || swap != swapHelper {
		t.Fatalf("writable bundle = %q %v", target, swap)
	}
	if os.Getuid() == 0 {
		t.Skip("root writes anywhere")
	}
	// A standard account cannot write to /Applications, but it owns a
	// bundle that it installed. The swap then replaces the Contents folder
	// of the bundle and needs no administrator prompt.
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if target, swap := updateTarget("darwin", exe, ""); target != bundle || swap != swapContents {
		t.Fatalf("own bundle in a locked folder = %q %v", target, swap)
	}
	// A bundle that the account cannot write needs the administrator
	// prompt, such as a bundle that root owns.
	if err := os.Chmod(bundle, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(bundle, 0o755) })
	if target, swap := updateTarget("darwin", exe, ""); target != bundle || swap != swapElevated {
		t.Fatalf("locked bundle in a locked folder = %q %v", target, swap)
	}
	// On Windows the executable's folder decides, such as Program Files.
	if target, swap := updateTarget("windows", filepath.Join(dir, "rolle.exe"), ""); target != filepath.Join(dir, "rolle.exe") || swap != swapElevated {
		t.Fatalf("executable in a locked folder = %q %v", target, swap)
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

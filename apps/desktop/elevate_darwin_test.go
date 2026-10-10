//go:build darwin

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDarwinSwapAndRelaunchCommands(t *testing.T) {
	swap := darwinSwapCommand("/tmp/wails-update-1/rolle.app", "/Applications/rolle.app")
	want := `d=$(mktemp -d '/Applications/.rolle-update.XXXXXX') || exit 1; ditto '/tmp/wails-update-1/rolle.app' "$d/new.app" && chown -R root:wheel "$d/new.app" && chmod -R go-w "$d/new.app" && codesign --verify --deep --strict -R '=anchor apple generic and identifier "com.getrolle.app" and certificate leaf[subject.OU] = "AMR56F4NQB"' "$d/new.app" && mv '/Applications/rolle.app' "$d/old.app" && mv "$d/new.app" '/Applications/rolle.app' || { mv "$d/old.app" '/Applications/rolle.app' 2>/dev/null; rm -rf "$d"; exit 1; }; rm -rf "$d"`
	if swap != want {
		t.Fatalf("swap = %s\nwant   %s", swap, want)
	}
	if out, err := exec.Command("/bin/sh", "-n", "-c", darwinSwapCommand("/tmp/it's here/rolle.app", "/Applications/my apps/rolle.app")).CombinedOutput(); err != nil {
		t.Fatalf("sh -n: %v\n%s", err, out)
	}
	if got := darwinRelaunchCommand(42, "/Applications/rolle.app"); got != "while kill -0 42 2>/dev/null; do sleep 0.2; done; open '/Applications/rolle.app'" {
		t.Fatalf("relaunch = %s", got)
	}
}

// TestDarwinSwapCommandRuns runs the swap line without root. Stubs replace
// chown, which needs root, and codesign, which needs a signed bundle. The
// codesign stub fails when the CODESIGN_FAIL variable is set.
func TestDarwinSwapCommandRuns(t *testing.T) {
	stubs := t.TempDir()
	for name, body := range map[string]string{
		"chown":    "#!/bin/sh\nexit 0\n",
		"codesign": "#!/bin/sh\n[ -z \"$CODESIGN_FAIL\" ]\n",
	} {
		if err := os.WriteFile(filepath.Join(stubs, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	bundle := func(dir, version string) string {
		app := filepath.Join(dir, "rolle.app")
		if err := os.MkdirAll(filepath.Join(app, "Contents"), 0o755); err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(app, "Contents", "version")
		if err := os.WriteFile(file, []byte(version), 0o644); err != nil {
			t.Fatal(err)
		}
		// The umask removes write access for others on create. Set it
		// after, so the swap must remove it.
		if err := os.Chmod(file, 0o666); err != nil {
			t.Fatal(err)
		}
		return app
	}
	run := func(staged, target string, fail bool) error {
		cmd := exec.Command("/bin/sh", "-c", darwinSwapCommand(staged, target))
		cmd.Env = append(os.Environ(), "PATH="+stubs+":/usr/bin:/bin")
		if fail {
			cmd.Env = append(cmd.Env, "CODESIGN_FAIL=1")
		}
		return cmd.Run()
	}
	installed := func(target string) string {
		b, err := os.ReadFile(filepath.Join(target, "Contents", "version"))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	leftovers := func(dir string) {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".rolle-update.") {
				t.Fatalf("temporary folder left behind: %s", e.Name())
			}
		}
	}

	apps := t.TempDir()
	target := bundle(apps, "old")
	staged := bundle(t.TempDir(), "new")

	if err := run(staged, target, true); err == nil {
		t.Fatal("swap passed a failed signature check")
	}
	if got := installed(target); got != "old" {
		t.Fatalf("failed check replaced the bundle: %s", got)
	}
	leftovers(apps)

	if err := run(staged, target, false); err != nil {
		t.Fatal(err)
	}
	if got := installed(target); got != "new" {
		t.Fatalf("installed = %s", got)
	}
	info, err := os.Stat(filepath.Join(target, "Contents", "version"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o022 != 0 {
		t.Fatalf("installed file is writable by others: %v", info.Mode())
	}
	leftovers(apps)
}

// TestContentsSwapNeedsNoWriteAccessToTheFolder replaces the Contents folder
// of a bundle in a folder that refuses writes, like /Applications for a
// standard account. A stub replaces codesign, which needs a signed bundle.
// The stub fails when the CODESIGN_FAIL variable is set.
func TestContentsSwapNeedsNoWriteAccessToTheFolder(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root writes anywhere")
	}
	stubs := t.TempDir()
	if err := os.WriteFile(filepath.Join(stubs, "codesign"), []byte("#!/bin/sh\n[ -z \"$CODESIGN_FAIL\" ]\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubs+":/usr/bin:/bin")
	bundle := func(dir, version string) string {
		app := filepath.Join(dir, "rolle.app")
		if err := os.MkdirAll(filepath.Join(app, "Contents"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(app, "Contents", "version"), []byte(version), 0o644); err != nil {
			t.Fatal(err)
		}
		return app
	}
	installed := func(target string) string {
		b, err := os.ReadFile(filepath.Join(target, "Contents", "version"))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	entries := func(dir string) []string {
		list, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, e := range list {
			names = append(names, e.Name())
		}
		return names
	}

	apps := t.TempDir()
	target := bundle(apps, "old")
	if err := os.Chmod(apps, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(apps, 0o755) })
	staging := t.TempDir()
	staged := bundle(staging, "new")

	t.Setenv("CODESIGN_FAIL", "1")
	if err := contentsSwap(staged, target); err == nil {
		t.Fatal("swap passed a failed signature check")
	}
	if got := installed(target); got != "old" {
		t.Fatalf("failed check replaced the bundle: %s", got)
	}
	t.Setenv("CODESIGN_FAIL", "")

	// A staged bundle without Contents fails after the old Contents moved
	// aside. The swap must move it back.
	empty := filepath.Join(t.TempDir(), "rolle.app")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := contentsSwap(empty, target); err == nil {
		t.Fatal("swap passed a bundle without Contents")
	}
	if got := installed(target); got != "old" {
		t.Fatalf("failed move lost the old bundle: %s", got)
	}
	if got := entries(filepath.Dir(empty)); len(got) != 1 {
		t.Fatalf("staging folder after rollback = %v", got)
	}

	if err := contentsSwap(staged, target); err != nil {
		t.Fatal(err)
	}
	if got := installed(target); got != "new" {
		t.Fatalf("installed = %s", got)
	}
	if got := entries(target); len(got) != 1 || got[0] != "Contents" {
		t.Fatalf("bundle holds %v, want only Contents", got)
	}
	if got := entries(staging); len(got) != 1 || got[0] != "rolle.app" {
		t.Fatalf("old Contents left in staging: %v", got)
	}
}

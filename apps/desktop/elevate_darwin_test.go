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
		if err := os.WriteFile(filepath.Join(app, "Contents", "version"), []byte(version), 0o666); err != nil {
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

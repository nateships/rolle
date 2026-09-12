package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/nateships/rolle/internal/version"
)

// fakeBundle writes a macOS app bundle with a helper under dir and returns
// the executable path inside it.
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

func notFound(string) (string, error) { return "", errors.New("not found") }

func TestDevBuildSimulatesCLIInstall(t *testing.T) {
	if !devMode {
		t.Skip("production build")
	}
	// The test binary is not in a bundle and carries no payload, so the dev
	// simulation is on.
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

func TestDarwinStatus(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bundle paths use forward slashes; the code runs on macOS only")
	}
	env := cliEnv{goos: "darwin", lookPath: notFound}
	env.exe = "/usr/local/bin/rolle-desktop"
	if st := cliStatusIn(env); st.Reason != "unsupported" {
		t.Fatalf("outside a bundle = %+v", st)
	}
	// A bundle without the helper is a dev bundle.
	dir := t.TempDir()
	env.exe = filepath.Join(dir, "rolle.app", "Contents", "MacOS", "rolle")
	if st := cliStatusIn(env); st.Reason != "unsupported" {
		t.Fatalf("bundle without helper = %+v", st)
	}

	env.exe = fakeBundle(t, dir)
	st := cliStatusIn(env)
	if st.Reason != "" || st.Target != filepath.Join(dir, "rolle.app", "Contents", "Helpers", "rolle") {
		t.Fatalf("status = %+v", st)
	}
	_, linkErr := os.Lstat(cliLink)
	if st.Installed != (linkErr == nil) {
		t.Fatalf("installed = %v, link present = %v", st.Installed, linkErr == nil)
	}
	env.lookPath = func(string) (string, error) { return "/opt/homebrew/bin/rolle", nil }
	if st := cliStatusIn(env); !st.Installed || st.Path != "/opt/homebrew/bin/rolle" {
		t.Fatalf("status with rolle on PATH = %+v", st)
	}
	env.exe = fakeBundle(t, filepath.Join(dir, "Downloads"))
	if st := cliStatusIn(env); st.Reason != "move" {
		t.Fatalf("status from Downloads = %+v", st)
	}
}

func TestUserDirStatusNeedsAPayload(t *testing.T) {
	for _, goos := range []string{"windows", "linux"} {
		env := cliEnv{goos: goos, home: t.TempDir(), localApp: t.TempDir(), lookPath: notFound}
		if st := cliStatusIn(env); st.Reason != "unsupported" {
			t.Fatalf("%s without payload = %+v", goos, st)
		}
	}
	if st := cliStatusIn(cliEnv{goos: "plan9", payload: []byte("x")}); st.Reason != "unsupported" {
		t.Fatalf("unknown OS = %+v", st)
	}
}

func TestWindowsStatusReadsTheUserPath(t *testing.T) {
	localApp := t.TempDir()
	target := filepath.Join(localApp, "rolle", "bin", "rolle.exe")
	userPath := `C:\Windows\system32;C:\Windows`
	env := cliEnv{
		goos:      "windows",
		localApp:  localApp,
		payload:   []byte("MZ"),
		lookPath:  notFound,
		versionOf: func(string) string { return version.Version },
		userPath:  func() string { return userPath },
	}
	st := cliStatusIn(env)
	if st.Installed || st.Reason != "" || st.Target != target {
		t.Fatalf("before install = %+v", st)
	}
	// Install writes the file and the PATH; this process still cannot see it.
	if err := writeCLI(target, env.payload); err != nil {
		t.Fatal(err)
	}
	userPath, _ = pathListAdd(userPath, filepath.Dir(target))
	st = cliStatusIn(env)
	if !st.Installed || st.Path != target || st.Note != "Open a new terminal to use it." {
		t.Fatalf("after install = %+v", st)
	}
	// A new terminal resolves it through PATH; no note then.
	env.lookPath = func(string) (string, error) { return target, nil }
	if st := cliStatusIn(env); !st.Installed || st.Note != "" {
		t.Fatalf("resolved = %+v", st)
	}
}

func TestLinuxStatusAndOutdatedCommand(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, ".local", "bin", "rolle")
	env := cliEnv{
		goos:      "linux",
		home:      home,
		payload:   []byte("ELF"),
		lookPath:  notFound,
		versionOf: func(string) string { return version.Version },
	}
	if st := cliStatusIn(env); st.Installed || st.Target != target || st.Note != "" {
		t.Fatalf("before install = %+v", st)
	}
	if err := writeCLI(target, env.payload); err != nil {
		t.Fatal(err)
	}
	if st := cliStatusIn(env); st.Installed || st.Note != "Add ~/.local/bin to your PATH." {
		t.Fatalf("file present, PATH without it = %+v", st)
	}
	env.lookPath = func(string) (string, error) { return target, nil }
	if st := cliStatusIn(env); !st.Installed || st.Path != target || st.Reason != "" {
		t.Fatalf("on PATH = %+v", st)
	}
	// A deb-installed command elsewhere counts as installed too.
	env.lookPath = func(string) (string, error) { return "/usr/local/bin/rolle", nil }
	if st := cliStatusIn(env); !st.Installed || st.Path != "/usr/local/bin/rolle" {
		t.Fatalf("system command = %+v", st)
	}
	// Another version is flagged, unless this is a dev build with no version.
	env.versionOf = func(string) string { return "0.0.9" }
	st := cliStatusIn(env)
	if want := version.Version != "0.0.1-dev"; (st.Reason == "outdated") != want {
		t.Fatalf("outdated = %+v (build version %s)", st, version.Version)
	}
}

func TestWriteCLIReplacesAtomically(t *testing.T) {
	target := filepath.Join(t.TempDir(), "bin", "rolle")
	if err := writeCLI(target, []byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := writeCLI(target, []byte("two")); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "two" {
		t.Fatalf("content = %q", got)
	}
	if _, err := os.Stat(target + ".tmp"); err == nil {
		t.Fatal("temporary file left behind")
	}
	if info, _ := os.Stat(target); runtime.GOOS != "windows" && info.Mode()&0o100 == 0 {
		t.Fatalf("not executable: %v", info.Mode())
	}
}

func TestPathList(t *testing.T) {
	list, changed := pathListAdd(`C:\Windows;`, `C:\Users\n\AppData\Local\rolle\bin`)
	if !changed || list != `C:\Windows;C:\Users\n\AppData\Local\rolle\bin` {
		t.Fatalf("add = %q %v", list, changed)
	}
	if _, changed := pathListAdd(list, `c:\users\n\appdata\local\ROLLE\bin`); changed {
		t.Fatal("add must be case-insensitive")
	}
	if list, changed := pathListAdd("", `D:\bin`); !changed || list != `D:\bin` {
		t.Fatalf("add to empty = %q", list)
	}
	list, changed = pathListRemove(list, `C:\Users\n\AppData\Local\rolle\bin`)
	if !changed || list != `C:\Windows` {
		t.Fatalf("remove = %q %v", list, changed)
	}
	if _, changed := pathListRemove(list, `D:\nope`); changed {
		t.Fatal("remove of an absent entry reports a change")
	}
}

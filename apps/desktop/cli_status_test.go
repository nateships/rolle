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
	if p, err := userLookPath("rolle"); err == nil {
		// This machine has a command from elsewhere; the dev build reports it.
		if st := r.CLIStatus(); !st.Installed || st.Path != p || st.Reason != "external" {
			t.Fatalf("with %s on the PATH = %+v", p, st)
		}
		return
	}
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
	// A Homebrew command is installed, but not ours to link or update.
	env.lookPath = func(string) (string, error) { return "/opt/homebrew/bin/rolle", nil }
	if st := cliStatusIn(env); !st.Installed || st.Path != "/opt/homebrew/bin/rolle" || st.Reason != "external" {
		t.Fatalf("status with rolle on PATH = %+v", st)
	}
	env.lookPath = func(string) (string, error) { return cliLink, nil }
	if st := cliStatusIn(env); !st.Installed || st.Reason != "" {
		t.Fatalf("status with our link on PATH = %+v", st)
	}
	env.lookPath = notFound
	env.exe = fakeBundle(t, filepath.Join(dir, "Downloads"))
	if st := cliStatusIn(env); st.Reason != "move" {
		t.Fatalf("status from Downloads = %+v", st)
	}
}

// fakeCaskroom makes a Caskroom with a rolle cask and returns its root.
func fakeCaskroom(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "rolle"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestDarwinStatusWithHomebrewCask(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bundle paths use forward slashes; the code runs on macOS only")
	}
	// On Intel Macs the cask links /usr/local/bin/rolle, the same path as the app.
	env := cliEnv{goos: "darwin", exe: fakeBundle(t, t.TempDir())}
	env.lookPath = func(string) (string, error) { return cliLink, nil }
	env.caskRoots = []string{t.TempDir(), fakeCaskroom(t)}
	if st := cliStatusIn(env); !st.Installed || st.Path != cliLink || st.Reason != "external" {
		t.Fatalf("status with the cask installed = %+v", st)
	}
	env.caskRoots = []string{t.TempDir()}
	if st := cliStatusIn(env); !st.Installed || st.Reason != "" {
		t.Fatalf("status without the cask = %+v", st)
	}
	// With no command, the cask does not stop an install.
	env.caskRoots = []string{fakeCaskroom(t)}
	env.lookPath = notFound
	if _, err := os.Lstat(cliLink); err == nil {
		return
	}
	if st := cliStatusIn(env); st.Installed || st.Reason != "" {
		t.Fatalf("status with the cask and no command = %+v", st)
	}
}

func TestCaskUnderOtherPrefixKeepsAppLink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows; the code runs on macOS only")
	}
	// Only a cask under /usr/local links /usr/local/bin/rolle. A cask under
	// /opt/homebrew links /opt/homebrew/bin/rolle.
	if got := liveEnv().caskRoots; len(got) != 1 || got[0] != "/usr/local/Caskroom" {
		t.Fatalf("caskRoots = %v, want [/usr/local/Caskroom]", got)
	}

	// An Apple silicon cask and a link that the app made in /usr/local/bin.
	brew := filepath.Join(t.TempDir(), "opt", "homebrew")
	if err := os.MkdirAll(filepath.Join(brew, "Caskroom", "rolle"), 0o755); err != nil {
		t.Fatal(err)
	}
	usrLocal := filepath.Join(t.TempDir(), "usr", "local")
	if err := os.MkdirAll(filepath.Join(usrLocal, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := cliEnv{goos: "darwin", exe: fakeBundle(t, t.TempDir())}
	env.caskRoots = []string{filepath.Join(usrLocal, "Caskroom")}
	env.lookPath = func(string) (string, error) { return cliLink, nil }
	if st := cliStatusIn(env); !st.Installed || st.Reason != "" {
		t.Fatalf("status with a cask under /opt/homebrew = %+v", st)
	}
	link := filepath.Join(usrLocal, "bin", "rolle")
	if err := os.Symlink(cliTarget(env.exe), link); err != nil {
		t.Fatal(err)
	}
	if err := uninstallDarwinLink(env, link); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("link still there: %v", err)
	}
}

func TestUninstallDarwinLink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows; the code runs on macOS only")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "rolle.app", "Contents", "Helpers", "rolle")
	link := filepath.Join(dir, "rolle")
	makeLink := func() {
		t.Helper()
		_ = os.Remove(link)
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
	}

	// No link: nothing to do.
	if err := uninstallDarwinLink(cliEnv{}, link); err != nil {
		t.Fatalf("missing link: %v", err)
	}

	// A link that Homebrew made stays.
	makeLink()
	if err := uninstallDarwinLink(cliEnv{caskRoots: []string{fakeCaskroom(t)}}, link); err == nil {
		t.Fatal("removed the link of the Homebrew cask")
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("link of the Homebrew cask is gone: %v", err)
	}

	// The app made only a link, so a regular file stays.
	_ = os.Remove(link)
	if err := os.WriteFile(link, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := uninstallDarwinLink(cliEnv{}, link); err == nil {
		t.Fatal("removed a regular file")
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("regular file is gone: %v", err)
	}

	// A link without a cask is the app's link.
	makeLink()
	if err := uninstallDarwinLink(cliEnv{caskRoots: []string{t.TempDir()}}, link); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("link still there: %v", err)
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
		userPath:  func() (string, error) { return userPath, nil },
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
	// Another version of our copy is flagged, unless this is a dev build with no version.
	env.versionOf = func(string) string { return "0.0.9" }
	st := cliStatusIn(env)
	if want := version.Version != "0.0.1-dev"; (st.Reason == "outdated") != want {
		t.Fatalf("outdated = %+v (build version %s)", st, version.Version)
	}
	// A deb-installed command elsewhere counts as installed, is not ours, and
	// is never called outdated.
	env.lookPath = func(string) (string, error) { return "/usr/local/bin/rolle", nil }
	if st := cliStatusIn(env); !st.Installed || st.Path != "/usr/local/bin/rolle" || st.Reason != "external" {
		t.Fatalf("system command = %+v", st)
	}
}

func TestUserLookPathAsksTheShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shell lookup is for macOS and Linux")
	}
	// sh is on every PATH, so both the direct and the shell route find it.
	if p, err := userLookPath("sh"); err != nil || !filepath.IsAbs(p) {
		t.Fatalf("sh = %q, %v", p, err)
	}
	if _, err := userLookPath("rolle-no-such-command-xyz"); err == nil {
		t.Fatal("a missing command was found")
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

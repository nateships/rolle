package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/nateships/rolle/internal/version"
)

func TestCommandVersionReadsTheLastField(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh scripts")
	}
	dir := t.TempDir()
	script := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if got := commandVersion(script("rolle", "echo rolle version 1.2.3\n")); got != "1.2.3" {
		t.Fatalf("version = %q", got)
	}
	// A command that prints nothing, fails, or is missing has no version,
	// so the status never calls it outdated.
	if got := commandVersion(script("quiet", "")); got != "" {
		t.Fatalf("quiet command version = %q", got)
	}
	if got := commandVersion(script("broken", "echo 0.0.9; exit 1\n")); got != "" {
		t.Fatalf("failing command version = %q", got)
	}
	if got := commandVersion(filepath.Join(dir, "missing")); got != "" {
		t.Fatalf("missing command version = %q", got)
	}
}

func TestWindowsStatusWithAnUnreadablePathIsNotInstalled(t *testing.T) {
	localApp := t.TempDir()
	target := filepath.Join(localApp, "rolle", "bin", "rolle.exe")
	if err := writeCLI(target, []byte("MZ")); err != nil {
		t.Fatal(err)
	}
	env := cliEnv{
		goos:      "windows",
		localApp:  localApp,
		payload:   []byte("MZ"),
		lookPath:  notFound,
		versionOf: func(string) string { return version.Version },
		userPath:  func() (string, error) { return "", errors.New("registry closed") },
	}
	if pathListHasIn(env, filepath.Dir(target)) {
		t.Fatal("an unreadable PATH names the directory")
	}
	// The file is there, but nothing shows the PATH has it: not installed,
	// and no hint about a new terminal.
	if st := cliStatusIn(env); st.Installed || st.Note != "" || st.Target != target {
		t.Fatalf("status = %+v", st)
	}
	env.userPath = func() (string, error) { return `C:\Windows;` + filepath.Dir(target), nil }
	if !pathListHasIn(env, filepath.Dir(target)) {
		t.Fatal("PATH with the directory not seen")
	}
}

func TestUserCLIPathNeedsAHomeDirectory(t *testing.T) {
	for _, env := range []cliEnv{
		{goos: "windows", payload: []byte("MZ"), lookPath: notFound},
		{goos: "linux", payload: []byte("ELF"), lookPath: notFound},
		{goos: "darwin", home: "/Users/n", localApp: `C:\Users\n\AppData\Local`},
	} {
		if p := userCLIPath(env); p != "" {
			t.Fatalf("%s without a home = %q", env.goos, p)
		}
	}
	// Without a place to write, a payload alone does not make an install possible.
	if st := cliStatusIn(cliEnv{goos: "windows", payload: []byte("MZ"), lookPath: notFound}); st.Reason != "unsupported" {
		t.Fatalf("windows without LOCALAPPDATA = %+v", st)
	}
	if st := cliStatusIn(cliEnv{goos: "linux", payload: []byte("ELF"), lookPath: notFound}); st.Reason != "unsupported" {
		t.Fatalf("linux without HOME = %+v", st)
	}
}

func TestWriteCLIClearsAStaleAsideAndKeepsTheOldCommandOnFailure(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "bin", "rolle")
	if err := writeCLI(target, []byte("one")); err != nil {
		t.Fatal(err)
	}
	// An aside from an interrupted update does not block the next one.
	if err := os.WriteFile(target+".old", []byte("stale"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeCLI(target, []byte("two")); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(target); string(got) != "two" {
		t.Fatalf("content = %q", got)
	}
	if _, err := os.Stat(target + ".old"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("aside left behind: %v", err)
	}
	// A file where the directory should be is an error, not a partial write.
	blocked := filepath.Join(dir, "file")
	if err := os.WriteFile(blocked, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeCLI(filepath.Join(blocked, "rolle"), []byte("x")); err == nil {
		t.Fatal("write through a file accepted")
	}
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		return
	}
	// When the temporary file cannot be written, the installed command stays.
	if err := os.Chmod(filepath.Dir(target), 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(target), 0o755) })
	if err := writeCLI(target, []byte("three")); err == nil {
		t.Fatal("write into a read-only directory accepted")
	}
	if got, _ := os.ReadFile(target); string(got) != "two" {
		t.Fatalf("content after failed write = %q", got)
	}
}

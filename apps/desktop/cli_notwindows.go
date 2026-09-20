//go:build !windows

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func userPathList() (string, error) { return "", nil }
func setUserPathList(string) error  { return nil }
func hideWindow(*exec.Cmd)          {}

// userLookPath resolves file the way the user's terminal does. An app opened
// from Finder or a launcher gets the system PATH, which leaves out Homebrew
// and ~/.local/bin, so when that PATH has no answer the login shell gives one.
//
// The shell is a login shell, not an interactive one. The rc file that -i
// reads starts completions, plugins, and daemons (1Password's `op daemon`
// is one), and every process it leaves behind stays in this app's process
// coalition. macOS then shows the app as "Running in Background" after it
// quits. The profile that -l reads is where Homebrew and most installers
// put PATH.
func userLookPath(file string) (string, error) {
	if p, err := exec.LookPath(file); err == nil {
		return p, nil
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, shell, "-lc", "command -v "+file).Output()
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	p := strings.TrimSpace(lines[len(lines)-1])
	if !filepath.IsAbs(p) {
		return "", exec.ErrNotFound
	}
	if _, err := os.Stat(p); err != nil {
		return "", err
	}
	return p, nil
}

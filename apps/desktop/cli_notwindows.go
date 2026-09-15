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
	// -i reads the rc file, where PATH usually grows; -l the profile.
	out, err := exec.CommandContext(ctx, shell, "-ilc", "command -v "+file).Output()
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

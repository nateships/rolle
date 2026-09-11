package gcp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// InstallURL documents how to install the gcloud CLI.
const InstallURL = "https://cloud.google.com/sdk/docs/install"

// ErrGCloudMissing reports that the lookup found no working gcloud CLI.
var ErrGCloudMissing = errors.New("gcp: gcloud CLI not found; install it from " + InstallURL)

// FindGCloud locates a runnable gcloud binary. A PATH hit wins unless it is a
// version-manager shim, which may not work outside an activated shell; then
// well-known install locations are tried, and the shim is the last resort.
func FindGCloud() (string, error) {
	var shim string
	if p, err := exec.LookPath(gcloudName()); err == nil {
		if !isShim(p) {
			return p, nil
		}
		shim = p
	}
	for _, c := range candidates() {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c, nil
		}
	}
	if shim != "" {
		return shim, nil
	}
	return "", ErrGCloudMissing
}

func gcloudName() string {
	if runtime.GOOS == "windows" {
		return "gcloud.cmd"
	}
	return "gcloud"
}

// isShim reports whether p lives in a version manager's shims directory.
// Paths are normalised so the check behaves the same on every platform.
func isShim(p string) bool {
	return strings.Contains(filepath.ToSlash(p), "/shims/")
}

// candidates lists common gcloud install paths, newest mise install first.
func candidates() []string {
	home, _ := os.UserHomeDir()
	var out []string
	if runtime.GOOS == "windows" {
		for _, base := range []string{os.Getenv("LOCALAPPDATA"), os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles")} {
			if base != "" {
				out = append(out, filepath.Join(base, "Google", "Cloud SDK", "google-cloud-sdk", "bin", "gcloud.cmd"))
			}
		}
		return out
	}
	if matches, _ := filepath.Glob(filepath.Join(home, ".local", "share", "mise", "installs", "gcloud", "*", "bin", "gcloud")); len(matches) > 0 {
		sort.Sort(sort.Reverse(sort.StringSlice(matches)))
		out = append(out, matches...)
	}
	out = append(out,
		"/opt/homebrew/bin/gcloud",
		"/opt/homebrew/share/google-cloud-sdk/bin/gcloud",
		"/usr/local/bin/gcloud",
		"/usr/local/share/google-cloud-sdk/bin/gcloud",
		filepath.Join(home, "google-cloud-sdk", "bin", "gcloud"),
		"/snap/bin/gcloud",
		"/usr/bin/gcloud",
	)
	return out
}

// GCloudLogin runs `gcloud auth application-default login`, which opens the
// browser and writes the Application Default Credentials file. It blocks until
// gcloud exits.
func GCloudLogin(ctx context.Context) error {
	path, err := FindGCloud()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, path, "auth", "application-default", "login", "--quiet")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gcloud login: %s", summarize(stderr.String(), err))
	}
	return nil
}

// summarize picks the most useful line out of gcloud's stderr.
func summarize(stderr string, err error) string {
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	for _, l := range lines {
		if strings.Contains(l, "ERROR") {
			return strings.TrimSpace(l)
		}
	}
	if last := strings.TrimSpace(lines[len(lines)-1]); last != "" {
		return last
	}
	return err.Error()
}

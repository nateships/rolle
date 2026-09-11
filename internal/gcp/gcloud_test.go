package gcp

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeExecutable creates an empty runnable file named like the gcloud binary.
func fakeExecutable(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, gcloudName())
	if err := os.WriteFile(path, []byte("@echo off\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestGCloudName(t *testing.T) {
	want := "gcloud"
	if runtime.GOOS == "windows" {
		want = "gcloud.cmd"
	}
	if got := gcloudName(); got != want {
		t.Fatalf("gcloudName = %q, want %q", got, want)
	}
}

func TestCandidates(t *testing.T) {
	home, _ := fakeHome(t)
	if runtime.GOOS == "windows" {
		local := filepath.Join(home, "local")
		t.Setenv("LOCALAPPDATA", local)
		t.Setenv("ProgramFiles", "")
		t.Setenv("ProgramFiles(x86)", "")
		want := []string{filepath.Join(local, "Google", "Cloud SDK", "google-cloud-sdk", "bin", "gcloud.cmd")}
		if got := candidates(); len(got) != 1 || got[0] != want[0] {
			t.Fatalf("candidates = %v, want %v", got, want)
		}
		return
	}
	for _, v := range []string{"400.0.0", "510.0.0"} {
		fakeExecutable(t, filepath.Join(home, ".local", "share", "mise", "installs", "gcloud", v, "bin"))
	}
	got := candidates()
	if len(got) < 3 || !strings.HasSuffix(got[0], filepath.Join("510.0.0", "bin", "gcloud")) || !strings.HasSuffix(got[1], filepath.Join("400.0.0", "bin", "gcloud")) {
		t.Fatalf("mise installs should come first, newest first: %v", got)
	}
	joined := strings.Join(got, "\n")
	for _, want := range []string{"/opt/homebrew/bin/gcloud", "/usr/local/bin/gcloud", filepath.Join(home, "google-cloud-sdk", "bin", "gcloud"), "/usr/bin/gcloud"} {
		if !strings.Contains(joined, want) {
			t.Errorf("candidates missing %q", want)
		}
	}
}

func TestFindGCloudPrefersPathHit(t *testing.T) {
	fakeHome(t)
	want := fakeExecutable(t, filepath.Join(t.TempDir(), "bin"))
	t.Setenv("PATH", filepath.Dir(want))
	got, err := FindGCloud()
	if err != nil || got != want {
		t.Fatalf("FindGCloud = %q, %v; want %q", got, err, want)
	}
}

func TestFindGCloudSkipsShimWhenAnInstallExists(t *testing.T) {
	fakeHome(t)
	shim := fakeExecutable(t, filepath.Join(t.TempDir(), "shims"))
	t.Setenv("PATH", filepath.Dir(shim))
	// The shim is the last resort: the first installed candidate wins.
	want := shim
	for _, c := range candidates() {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			want = c
			break
		}
	}
	got, err := FindGCloud()
	if err != nil || got != want {
		t.Fatalf("FindGCloud = %q, %v; want %q", got, err, want)
	}
}

func TestFindGCloudMissing(t *testing.T) {
	fakeHome(t)
	t.Setenv("PATH", t.TempDir())
	for _, c := range candidates() {
		if _, err := os.Stat(c); err == nil {
			t.Skipf("gcloud installed at %s", c)
		}
	}
	if _, err := FindGCloud(); !errors.Is(err, ErrGCloudMissing) {
		t.Fatalf("err = %v, want ErrGCloudMissing", err)
	}
}

func TestSummarizeFallsBackToLastLine(t *testing.T) {
	if got := summarize("first line\nsecond line\n\n", errors.New("exit 1")); got != "second line" {
		t.Fatalf("summarize = %q", got)
	}
	if got := summarize("  ERROR: (gcloud.auth) bad thing  \n", errors.New("exit 1")); got != "ERROR: (gcloud.auth) bad thing" {
		t.Fatalf("summarize = %q", got)
	}
}

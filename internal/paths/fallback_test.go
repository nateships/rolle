package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestXDGVariableBeatsExistingDotDir(t *testing.T) {
	home := fakeHome(t)
	dot := filepath.Join(home, ".config", "rolle")
	if err := os.MkdirAll(dot, 0o700); err != nil {
		t.Fatal(err)
	}
	xdg := filepath.Join(t.TempDir(), "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	if got := ConfigDir(); got != filepath.Join(xdg, "rolle") {
		t.Fatalf("ConfigDir = %q, want the XDG path", got)
	}
	// The variable is used as given, even when the directory does not exist.
	if _, err := os.Stat(xdg); err == nil {
		t.Fatal("ConfigDir must not create directories")
	}
}

func TestMissingHomeFallsBackToWorkingDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("USERPROFILE has more fallbacks on Windows")
	}
	fakeHome(t)
	t.Setenv("HOME", "")
	var wantCfg, wantCache string
	if runtime.GOOS == "darwin" {
		wantCfg = filepath.Join(".", "Library", "Application Support", "rolle")
		wantCache = filepath.Join(".", "Library", "Caches", "rolle")
	} else {
		wantCfg = filepath.Join(".", ".config", "rolle")
		wantCache = filepath.Join(".", ".cache", "rolle")
	}
	if got := ConfigDir(); got != wantCfg {
		t.Fatalf("ConfigDir = %q, want %q", got, wantCfg)
	}
	if got := CacheDir(); got != wantCache {
		t.Fatalf("CacheDir = %q, want %q", got, wantCache)
	}
}

func TestCacheDirKeepsExistingDotDir(t *testing.T) {
	home := fakeHome(t)
	dot := filepath.Join(home, ".cache", "rolle")
	if err := os.MkdirAll(dot, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := CacheDir(); got != dot {
		t.Fatalf("CacheDir = %q, want %q", got, dot)
	}
	// ConfigDir has its own dot directory and is not affected.
	if got := ConfigDir(); got == dot {
		t.Fatal("ConfigDir returned the cache path")
	}
}

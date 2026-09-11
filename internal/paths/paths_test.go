package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func fakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", "")
	t.Setenv("LOCALAPPDATA", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	return home
}

func TestXDGVariableWins(t *testing.T) {
	fakeHome(t)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "cfg"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	if got := ConfigDir(); got != filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "rolle") {
		t.Fatal(got)
	}
	if got := CacheDir(); got != filepath.Join(os.Getenv("XDG_CACHE_HOME"), "rolle") {
		t.Fatal(got)
	}
}

func TestExistingDotDirIsKept(t *testing.T) {
	home := fakeHome(t)
	dot := filepath.Join(home, ".config", "rolle")
	if err := os.MkdirAll(dot, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := ConfigDir(); got != dot {
		t.Fatalf("ConfigDir = %q, want %q", got, dot)
	}
	// A file with that name does not count. On Linux the fallback is the same
	// path, so the distinction is only visible elsewhere.
	if runtime.GOOS != "linux" {
		cache := filepath.Join(home, ".cache")
		if err := os.MkdirAll(cache, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cache, "rolle"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if got := CacheDir(); got == filepath.Join(cache, "rolle") {
			t.Fatal("a plain file was taken for the cache directory")
		}
	}
}

func TestPlatformDefault(t *testing.T) {
	home := fakeHome(t)
	var wantCfg, wantCache string
	switch runtime.GOOS {
	case "darwin":
		wantCfg = filepath.Join(home, "Library", "Application Support", "rolle")
		wantCache = filepath.Join(home, "Library", "Caches", "rolle")
	case "windows":
		t.Setenv("APPDATA", filepath.Join(home, "Roaming"))
		t.Setenv("LOCALAPPDATA", filepath.Join(home, "Local"))
		wantCfg = filepath.Join(home, "Roaming", "rolle")
		wantCache = filepath.Join(home, "Local", "rolle", "cache")
	default:
		wantCfg = filepath.Join(home, ".config", "rolle")
		wantCache = filepath.Join(home, ".cache", "rolle")
	}
	if got := ConfigDir(); got != wantCfg {
		t.Fatalf("ConfigDir = %q, want %q", got, wantCfg)
	}
	if got := CacheDir(); got != wantCache {
		t.Fatalf("CacheDir = %q, want %q", got, wantCache)
	}
}

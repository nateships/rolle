// Package paths resolves where Rolle keeps its files. The same rules run in
// the CLI and the desktop app, so both find one workspace even when only the
// shell has XDG variables set.
package paths

import (
	"os"
	"path/filepath"
	"runtime"
)

const app = "rolle"

// ConfigDir is the directory for the workspace file.
//
//  1. $XDG_CONFIG_HOME/rolle when the variable is set.
//  2. ~/.config/rolle when it already exists.
//  3. The platform default: ~/.config/rolle on Linux, ~/Library/Application
//     Support/rolle on macOS, %APPDATA%\rolle on Windows.
func ConfigDir() string {
	return resolve("XDG_CONFIG_HOME", ".config", func(home string) string {
		switch runtime.GOOS {
		case "darwin":
			return filepath.Join(home, "Library", "Application Support", app)
		case "windows":
			if d := os.Getenv("APPDATA"); d != "" {
				return filepath.Join(d, app)
			}
			return filepath.Join(home, "AppData", "Roaming", app)
		}
		return filepath.Join(home, ".config", app)
	})
}

// CacheDir is the directory for short-lived credentials. Same rules as
// ConfigDir with XDG_CACHE_HOME, ~/.cache, and the platform cache location.
func CacheDir() string {
	return resolve("XDG_CACHE_HOME", ".cache", func(home string) string {
		switch runtime.GOOS {
		case "darwin":
			return filepath.Join(home, "Library", "Caches", app)
		case "windows":
			if d := os.Getenv("LOCALAPPDATA"); d != "" {
				return filepath.Join(d, app, "cache")
			}
			return filepath.Join(home, "AppData", "Local", app, "cache")
		}
		return filepath.Join(home, ".cache", app)
	})
}

func resolve(envVar, dotDir string, platform func(home string) string) string {
	if base := os.Getenv(envVar); base != "" {
		return filepath.Join(base, app)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	if dot := filepath.Join(home, dotDir, app); isDir(dot) {
		return dot
	}
	return platform(home)
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/nateships/rolle/internal/debug"
	"github.com/nateships/rolle/internal/support"
	"github.com/nateships/rolle/internal/version"
)

// ExportSupportBundle writes the redacted support zip to the Downloads folder,
// shows it in the file manager, and returns its path.
func (r *RolleService) ExportSupportBundle() (string, error) {
	w, err := r.svc.Load()
	if err != nil {
		return "", err
	}
	st, _ := r.svc.Settings()
	path := support.DefaultPath(time.Now())
	in := support.Inputs{
		Version:       version.Version,
		App:           "desktop",
		Workspace:     w,
		Settings:      st,
		WorkspacePath: r.svc.WorkspacePath,
		CacheDir:      r.svc.Cache.Dir,
		AWSConfigPath: r.svc.AWSConfigPath,
		Log:           debug.Recent(),
	}
	if err := support.WriteFile(path, in); err != nil {
		return "", err
	}
	reveal(path)
	return path, nil
}

// reveal selects the file in the platform file manager. Failure is not an
// error; the path is shown in the interface as well.
func reveal(path string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", "-R", path)
	case "windows":
		cmd = exec.Command("explorer", "/select,"+path)
	default:
		cmd = exec.Command("xdg-open", filepath.Dir(path))
	}
	if err := cmd.Start(); err != nil {
		debug.Logf("support", "reveal %s: %v", path, err)
	}
}

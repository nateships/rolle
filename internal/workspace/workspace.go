// Package workspace loads and saves the rolle workspace file.
package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nateships/rolle/internal/atomicfile"
	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/paths"
)

// DefaultPath returns the workspace file location, honouring ROLLE_WORKSPACE.
func DefaultPath() string {
	if p := os.Getenv("ROLLE_WORKSPACE"); p != "" {
		return p
	}
	return filepath.Join(paths.ConfigDir(), "workspace.json")
}

// Load reads the workspace at path. A missing file yields an empty workspace.
func Load(path string) (*core.Workspace, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &core.Workspace{Version: core.WorkspaceVersion}, nil
	}
	if err != nil {
		return nil, err
	}
	var w core.Workspace
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if w.Version > core.WorkspaceVersion {
		return nil, fmt.Errorf("%s is version %d, this build supports up to %d", path, w.Version, core.WorkspaceVersion)
	}
	w.Version = core.WorkspaceVersion
	return &w, nil
}

// Save writes the workspace atomically with owner-only permissions.
func Save(path string, w *core.Workspace) error {
	w.Version = core.WorkspaceVersion
	// Computed for the window, not part of the file.
	w.ShadowedProfiles = nil
	data, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.Write(path, append(data, '\n'), 0o600)
}

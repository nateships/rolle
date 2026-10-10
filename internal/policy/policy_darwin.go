//go:build darwin

package policy

import (
	"os"
	"os/user"
	"path/filepath"
	"syscall"
)

// managedDir is where macOS puts the preferences of installed profiles.
const managedDir = "/Library/Managed Preferences"

// sources lists the profile files: the one for the computer, then the one for
// the current user, which wins.
func sources() []string {
	paths := []string{filepath.Join(managedDir, Domain+".plist")}
	if u, err := user.Current(); err == nil && u.Username != "" {
		paths = append(paths, filepath.Join(managedDir, u.Username, Domain+".plist"))
	}
	return paths
}

// trusted accepts a file that root owns and that only root can write. Only
// macOS writes there. The check stops a value that a user puts there some
// other way.
func trusted(fi os.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	return ok && st.Uid == 0 && fi.Mode().Perm()&0o022 == 0
}

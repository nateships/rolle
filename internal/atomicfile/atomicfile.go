// Package atomicfile writes a file through a temporary file and a rename, so
// a reader sees the old file or the new file, never a part of a file.
package atomicfile

import (
	"os"
	"path/filepath"
)

// Write puts data in the file at path with mode perm. It writes a temporary
// file in the same directory and renames it onto path, so a failed write
// leaves the old file in place. When path is a symlink, Write replaces the
// target file and the link stays a link. Missing parent directories get
// owner-only permissions.
func Write(path string, data []byte, perm os.FileMode) error {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".rolle-*")
	if err != nil {
		return err
	}
	err = tmp.Chmod(perm)
	if err == nil {
		_, err = tmp.Write(data)
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
	}
	return err
}

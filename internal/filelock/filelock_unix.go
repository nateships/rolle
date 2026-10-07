//go:build unix

package filelock

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// tryLock uses flock. A flock lock belongs to the open file, so two opens in
// one process also exclude each other.
func tryLock(f *os.File) (bool, error) {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) {
		return false, nil
	}
	return err == nil, err
}

func unlockFile(f *os.File) error { return unix.Flock(int(f.Fd()), unix.LOCK_UN) }

// Package filelock takes an exclusive lock on a file. The lock works across
// processes, so the desktop app and the CLI do not run the same critical
// section at the same time. The operating system releases the lock when the
// process stops.
package filelock

import (
	"context"
	"os"
	"path/filepath"
	"time"
)

// retry is how long Lock waits between two tries.
const retry = 25 * time.Millisecond

// Lock creates path if it does not exist and waits until it holds an
// exclusive lock on it. It stops when ctx ends. Call unlock to release the
// lock. The file stays: a removed file lets two processes lock two different
// files.
func Lock(ctx context.Context, path string) (unlock func(), err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	for {
		ok, err := tryLock(f)
		if err != nil {
			_ = f.Close()
			return nil, err
		}
		if ok {
			return func() {
				_ = unlockFile(f)
				_ = f.Close()
			}, nil
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, ctx.Err()
		case <-time.After(retry):
		}
	}
}

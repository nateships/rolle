package aws

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"time"

	"github.com/nateships/rolle/internal/debug"
	"github.com/nateships/rolle/internal/filelock"
)

// lockHold is the maximum time that one process holds a token lock. A refresh
// that hangs then stops, and the next process can try.
var lockHold = 30 * time.Second

// lockToken takes the cross-process lock of the token under key. Hold it from
// the read of a lapsed token to the store of the renewed one, so that two
// processes do not both spend one refresh token. Use the returned context for
// that work: it ends after lockHold. An empty dir takes no lock. When the lock
// file cannot be made, the renewal continues without the lock.
func lockToken(ctx context.Context, dir, key string) (context.Context, func(), error) {
	unlock := func() {}
	if dir != "" {
		held, err := filelock.Lock(ctx, tokenLockPath(dir, key))
		switch {
		case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
			return nil, nil, err
		case err != nil:
			debug.Logf("aws", "token lock for %s: %v", key, err)
		default:
			unlock = held
		}
	}
	ctx, cancel := context.WithTimeout(ctx, lockHold)
	return ctx, func() {
		cancel()
		unlock()
	}, nil
}

// tokenLockPath names the lock file of key. The key holds IDs from the
// workspace file, so the name is a hash and not the key.
func tokenLockPath(dir, key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(dir, hex.EncodeToString(sum[:16])+".lock")
}

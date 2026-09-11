// Package credcache stores short-lived credentials per session on disk with
// owner-only permissions, the same way the AWS CLI caches SSO credentials.
package credcache

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/adrg/xdg"

	"github.com/nateships/rolle/internal/core"
)

// ErrMiss is returned when no fresh credentials are cached.
var ErrMiss = errors.New("credential cache miss")

// Cache is a directory of credential files.
type Cache struct {
	Dir string
	// Skew is how long before expiry credentials count as stale. Default five minutes.
	Skew time.Duration
	Now  func() time.Time
}

// Default returns the cache under the XDG cache directory, honouring ROLLE_CACHE_DIR.
func Default() *Cache {
	dir := os.Getenv("ROLLE_CACHE_DIR")
	if dir == "" {
		dir = filepath.Join(xdg.CacheHome, "rolle", "credentials")
	}
	return &Cache{Dir: dir}
}

func (c *Cache) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Cache) skew() time.Duration {
	if c.Skew > 0 {
		return c.Skew
	}
	return 5 * time.Minute
}

func (c *Cache) path(sessionID string) string { return filepath.Join(c.Dir, sessionID+".json") }

// Get returns cached credentials if they are still fresh.
func (c *Cache) Get(sessionID string) (core.Credentials, error) {
	data, err := os.ReadFile(c.path(sessionID))
	if err != nil {
		return core.Credentials{}, ErrMiss
	}
	var creds core.Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return core.Credentials{}, ErrMiss
	}
	if creds.Expired(c.now(), c.skew()) {
		return core.Credentials{}, ErrMiss
	}
	return creds, nil
}

// Put stores credentials for a session.
func (c *Cache) Put(sessionID string, creds core.Credentials) error {
	if err := os.MkdirAll(c.Dir, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(creds)
	if err != nil {
		return err
	}
	// A unique temp file keeps concurrent credential_process calls apart.
	f, err := os.CreateTemp(c.Dir, sessionID+".*.tmp")
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return err
	}
	return os.Rename(f.Name(), c.path(sessionID))
}

// Delete removes cached credentials for a session.
func (c *Cache) Delete(sessionID string) error {
	err := os.Remove(c.path(sessionID))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

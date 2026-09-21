// Package credcache keeps the short-lived credentials of started sessions in
// the OS keychain, so credential_process can answer without a network round
// trip and no credential is written to disk. The cache directory holds only
// the files other tools read: launcher scripts and GCP ADC files.
package credcache

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/paths"
	"github.com/nateships/rolle/internal/secrets"
)

// ErrMiss is returned when no fresh credentials are cached.
var ErrMiss = errors.New("credential cache miss")

// Cache holds the credentials of started sessions.
type Cache struct {
	// Store holds the credentials, one entry per session. The OS keychain in
	// production.
	Store secrets.Store
	// Dir is a private scratch directory for the files other tools read.
	Dir string
	// Skew is how long before expiry credentials count as stale. Default five minutes.
	Skew time.Duration
	Now  func() time.Time
}

// Default returns the keychain cache with the scratch directory under the
// XDG cache directory, honouring ROLLE_CACHE_DIR.
func Default() *Cache {
	dir := os.Getenv("ROLLE_CACHE_DIR")
	if dir == "" {
		dir = filepath.Join(paths.CacheDir(), "credentials")
	}
	return &Cache{Store: secrets.NewKeychain(), Dir: dir}
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

func key(sessionID string) string { return "credentials/" + sessionID }

// Get returns cached credentials if they are still fresh. A stale entry is
// removed, so the keychain does not keep expired secrets.
func (c *Cache) Get(sessionID string) (core.Credentials, error) {
	raw, err := c.Store.Get(key(sessionID))
	if err != nil {
		return core.Credentials{}, ErrMiss
	}
	var creds core.Credentials
	if err := json.Unmarshal([]byte(raw), &creds); err != nil {
		return core.Credentials{}, ErrMiss
	}
	if creds.Expired(c.now(), c.skew()) {
		_ = c.Store.Delete(key(sessionID))
		return core.Credentials{}, ErrMiss
	}
	return creds, nil
}

// Put stores credentials for a session.
func (c *Cache) Put(sessionID string, creds core.Credentials) error {
	data, err := json.Marshal(creds)
	if err != nil {
		return err
	}
	return c.Store.Set(key(sessionID), string(data))
}

// Delete removes cached credentials for a session.
func (c *Cache) Delete(sessionID string) error {
	err := c.Store.Delete(key(sessionID))
	if errors.Is(err, secrets.ErrNotFound) {
		return nil
	}
	return err
}

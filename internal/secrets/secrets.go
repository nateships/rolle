// Package secrets stores long-lived secrets such as access keys and refresh
// tokens in the operating system keychain.
package secrets

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/zalando/go-keyring"
)

// ErrNotFound is returned when a key has no value.
var ErrNotFound = errors.New("secret not found")

// Store reads and writes secrets by key.
type Store interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
}

const service = "rolle"

// NewKeychain returns the OS keychain store. Values are chunked because the
// macOS backend rejects entries above a few kilobytes, and Identity Center
// tokens exceed that.
func NewKeychain() Store { return Chunked{Store: Keychain{}, Size: 2000} }

// Keychain stores secrets in the OS keychain. Use NewKeychain for large values.
type Keychain struct{}

// Get implements Store.
func (Keychain) Get(key string) (string, error) {
	v, err := keyring.Get(service, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return v, err
}

// Set implements Store.
func (Keychain) Set(key, value string) error { return keyring.Set(service, key, value) }

// Delete implements Store.
func (Keychain) Delete(key string) error {
	err := keyring.Delete(service, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

// Memory is an in-memory Store for tests.
type Memory struct {
	mu sync.Mutex
	m  map[string]string
}

// Get implements Store.
func (s *Memory) Get(key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[key]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

// Set implements Store.
func (s *Memory) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[string]string{}
	}
	s.m[key] = value
	return nil
}

// Delete implements Store.
func (s *Memory) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
	return nil
}

// Chunked splits values longer than Size across numbered entries so a
// backend with a per-entry limit can hold them. Small values are stored as is.
type Chunked struct {
	Store Store
	Size  int
}

const chunkMarker = "chunks:"

func chunkKey(key string, i int) string { return fmt.Sprintf("%s#%d", key, i) }

// Get implements Store.
func (c Chunked) Get(key string) (string, error) {
	head, err := c.Store.Get(key)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(head, chunkMarker) {
		return head, nil
	}
	n, err := strconv.Atoi(strings.TrimPrefix(head, chunkMarker))
	if err != nil {
		return "", fmt.Errorf("secrets: bad chunk marker for %s", key)
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		part, err := c.Store.Get(chunkKey(key, i))
		if err != nil {
			return "", fmt.Errorf("secrets: chunk %d of %s: %w", i, key, err)
		}
		b.WriteString(part)
	}
	return b.String(), nil
}

// Set implements Store.
func (c Chunked) Set(key, value string) error {
	if err := c.deleteChunks(key); err != nil {
		return err
	}
	if len(value) <= c.Size {
		return c.Store.Set(key, value)
	}
	n := 0
	for start := 0; start < len(value); start += c.Size {
		end := start + c.Size
		if end > len(value) {
			end = len(value)
		}
		if err := c.Store.Set(chunkKey(key, n), value[start:end]); err != nil {
			return err
		}
		n++
	}
	return c.Store.Set(key, chunkMarker+strconv.Itoa(n))
}

// Delete implements Store.
func (c Chunked) Delete(key string) error {
	if err := c.deleteChunks(key); err != nil {
		return err
	}
	return c.Store.Delete(key)
}

// deleteChunks removes chunk entries left by a previous large value.
func (c Chunked) deleteChunks(key string) error {
	head, err := c.Store.Get(key)
	if err != nil || !strings.HasPrefix(head, chunkMarker) {
		return nil
	}
	n, err := strconv.Atoi(strings.TrimPrefix(head, chunkMarker))
	if err != nil {
		return nil
	}
	for i := 0; i < n; i++ {
		if err := c.Store.Delete(chunkKey(key, i)); err != nil {
			return err
		}
	}
	return nil
}

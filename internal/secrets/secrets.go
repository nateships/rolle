// Package secrets stores long-lived secrets such as access keys and refresh
// tokens in the operating system keychain.
package secrets

import (
	"crypto/rand"
	"encoding/hex"
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
//
// The chunks of one value carry a generation tag, and the head names the
// generation. A rewrite stores the new chunks first and switches the head in
// one write, so a reader sees the old value or the new one, never a mix,
// when two processes write at the same time. Heads without a generation,
// from earlier versions, still read.
type Chunked struct {
	Store Store
	Size  int
}

const chunkMarker = "chunks:"

// chunkKey names chunk i of key. gen is empty for a head without a generation.
func chunkKey(key, gen string, i int) string {
	if gen == "" {
		return fmt.Sprintf("%s#%d", key, i)
	}
	return fmt.Sprintf("%s#%s#%d", key, gen, i)
}

// parseHead reads "chunks:<n>" or "chunks:<n>:<gen>". ok is false for a
// value that is not a chunk marker.
func parseHead(head string) (n int, gen string, ok bool, err error) {
	if !strings.HasPrefix(head, chunkMarker) {
		return 0, "", false, nil
	}
	count, gen, _ := strings.Cut(strings.TrimPrefix(head, chunkMarker), ":")
	n, err = strconv.Atoi(count)
	if err != nil {
		return 0, "", true, err
	}
	return n, gen, true, nil
}

func newGeneration() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// Get implements Store.
func (c Chunked) Get(key string) (string, error) {
	head, err := c.Store.Get(key)
	if err != nil {
		return "", err
	}
	n, gen, ok, err := parseHead(head)
	if !ok {
		return head, nil
	}
	if err != nil {
		return "", fmt.Errorf("secrets: bad chunk marker for %s", key)
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		part, err := c.Store.Get(chunkKey(key, gen, i))
		if err != nil {
			return "", fmt.Errorf("secrets: chunk %d of %s: %w", i, key, err)
		}
		b.WriteString(part)
	}
	return b.String(), nil
}

// Set implements Store.
func (c Chunked) Set(key, value string) error {
	// The previous head names the chunks to remove once the new value is in.
	prev, _ := c.Store.Get(key)
	if len(value) <= c.Size {
		if err := c.Store.Set(key, value); err != nil {
			return err
		}
		return c.deleteChunks(key, prev)
	}
	gen := newGeneration()
	n := 0
	for start := 0; start < len(value); start += c.Size {
		end := min(start+c.Size, len(value))
		if err := c.Store.Set(chunkKey(key, gen, n), value[start:end]); err != nil {
			return err
		}
		n++
	}
	if err := c.Store.Set(key, chunkMarker+strconv.Itoa(n)+":"+gen); err != nil {
		return err
	}
	return c.deleteChunks(key, prev)
}

// Delete implements Store.
func (c Chunked) Delete(key string) error {
	head, err := c.Store.Get(key)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	if err := c.deleteChunks(key, head); err != nil {
		return err
	}
	return c.Store.Delete(key)
}

// deleteChunks removes the chunks that head names. A head that is not a
// chunk marker, or a bad one, names nothing.
func (c Chunked) deleteChunks(key, head string) error {
	n, gen, ok, err := parseHead(head)
	if !ok || err != nil {
		return nil
	}
	for i := 0; i < n; i++ {
		if err := c.Store.Delete(chunkKey(key, gen, i)); err != nil {
			return err
		}
	}
	return nil
}

// Package secrets stores long-lived secrets such as access keys and refresh
// tokens in the operating system keychain.
package secrets

import (
	"errors"
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

// Keychain stores secrets in the OS keychain.
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

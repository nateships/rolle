package secrets

import (
	"errors"
	"testing"
)

// deleteFails serves a chunk marker but refuses to delete chunks.
type deleteFails struct {
	Memory
	err error
}

func (d *deleteFails) Delete(string) error { return d.err }

func TestNewKeychainChunksAtTwoThousandBytes(t *testing.T) {
	c, ok := NewKeychain().(Chunked)
	if !ok {
		t.Fatalf("NewKeychain returned %T, want Chunked", NewKeychain())
	}
	if c.Size != 2000 {
		t.Fatalf("chunk size = %d, want 2000", c.Size)
	}
	if _, ok := c.Store.(Keychain); !ok {
		t.Fatalf("inner store = %T, want Keychain", c.Store)
	}
}

func TestChunkedReportsFailedChunkCleanup(t *testing.T) {
	boom := errors.New("boom")
	inner := &deleteFails{err: boom}
	if err := inner.Set("k", "chunks:2"); err != nil {
		t.Fatal(err)
	}
	ch := Chunked{Store: inner, Size: 4}
	if err := ch.Set("k", "v"); !errors.Is(err, boom) {
		t.Fatalf("Set over stale chunks = %v, want the delete error", err)
	}
	if err := ch.Delete("k"); !errors.Is(err, boom) {
		t.Fatalf("Delete = %v, want the delete error", err)
	}
	// A plain value has no chunks to clean, so the head is deleted directly
	// and the store error still surfaces.
	if err := inner.Set("p", "plain"); err != nil {
		t.Fatal(err)
	}
	if err := ch.Delete("p"); !errors.Is(err, boom) {
		t.Fatalf("Delete plain = %v", err)
	}
}

func TestChunkedDeleteIgnoresBadMarker(t *testing.T) {
	mem := &Memory{}
	if err := mem.Set("k", "chunks:many"); err != nil {
		t.Fatal(err)
	}
	if err := (Chunked{Store: mem, Size: 2}).Delete("k"); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Get("k"); !errors.Is(err, ErrNotFound) {
		t.Fatal("head with a bad marker not deleted")
	}
}

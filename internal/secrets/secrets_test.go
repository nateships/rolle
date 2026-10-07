package secrets

import (
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
)

// chunkOf names chunk i of key under the generation the stored head names.
func chunkOf(t *testing.T, mem *Memory, key string, i int) string {
	t.Helper()
	head, err := mem.Get(key)
	if err != nil {
		t.Fatalf("head of %s: %v", key, err)
	}
	_, gen, ok, err := parseHead(head)
	if !ok || err != nil {
		t.Fatalf("head of %s = %q", key, head)
	}
	return chunkKey(key, gen, i)
}

// isChunked reports whether head marks n chunks.
func isChunked(head string, n int) bool {
	got, _, ok, err := parseHead(head)
	return ok && err == nil && got == n
}

// TestChunkedLegacyHead reads a value whose head has no generation, as
// earlier versions wrote it, and rewrites it into the tagged form.
func TestChunkedLegacyHead(t *testing.T) {
	mem := &Memory{}
	for k, v := range map[string]string{"k": "chunks:2", "k#0": "ab", "k#1": "cd"} {
		if err := mem.Set(k, v); err != nil {
			t.Fatal(err)
		}
	}
	c := Chunked{Store: mem, Size: 2}
	if got, err := c.Get("k"); err != nil || got != "abcd" {
		t.Fatalf("legacy Get = %q, %v", got, err)
	}
	if err := c.Set("k", "wxyz"); err != nil {
		t.Fatal(err)
	}
	if got, _ := c.Get("k"); got != "wxyz" {
		t.Fatalf("after rewrite = %q", got)
	}
	for _, old := range []string{"k#0", "k#1"} {
		if _, err := mem.Get(old); !errors.Is(err, ErrNotFound) {
			t.Fatalf("legacy chunk %s remains", old)
		}
	}
	if len(mem.m) != 3 {
		t.Fatalf("entries = %v", mem.m)
	}
}

// TestChunkedRewriteKeepsOldValueReadable checks that the head switches in
// one write: until it does, the old chunks are intact.
func TestChunkedRewriteKeepsOldValueReadable(t *testing.T) {
	mem := &Memory{}
	c := Chunked{Store: mem, Size: 2}
	if err := c.Set("k", "abcd"); err != nil {
		t.Fatal(err)
	}
	oldHead, _ := mem.Get("k")
	if err := c.Set("k", "wxyz"); err != nil {
		t.Fatal(err)
	}
	newHead, _ := mem.Get("k")
	if oldHead == newHead {
		t.Fatalf("head did not change generation: %q", newHead)
	}
	if got, _ := c.Get("k"); got != "wxyz" {
		t.Fatalf("Get = %q", got)
	}
	if len(mem.m) != 3 {
		t.Fatalf("old chunks not removed: %v", mem.m)
	}
}

func TestChunkedRoundTrip(t *testing.T) {
	mem := &Memory{}
	c := Chunked{Store: mem, Size: 10}
	big := strings.Repeat("abcdefghij", 5) + "tail"
	if err := c.Set("k", big); err != nil {
		t.Fatal(err)
	}
	got, err := c.Get("k")
	if err != nil || got != big {
		t.Fatalf("got %q, %v", got, err)
	}
	if head, _ := mem.Get("k"); !isChunked(head, 6) {
		t.Fatalf("marker = %q", head)
	}
	first := chunkOf(t, mem, "k", 0)
	// A smaller value replaces the chunks and removes the leftovers.
	if err := c.Set("k", "small"); err != nil {
		t.Fatal(err)
	}
	if got, _ := c.Get("k"); got != "small" {
		t.Fatalf("got %q", got)
	}
	if _, err := mem.Get(first); err != ErrNotFound {
		t.Fatal("stale chunk not removed")
	}
	if err := c.Set("k", big); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete("k"); err != nil {
		t.Fatal(err)
	}
	if len(mem.m) != 0 {
		t.Fatalf("entries after Delete: %v", mem.m)
	}
	if _, err := c.Get("missing"); err != ErrNotFound {
		t.Fatalf("missing key: %v", err)
	}
}

// rewriteOnChunkRead runs one rewrite of key just before the first chunk read.
// It puts a writer between the head read and the chunk reads of a reader.
type rewriteOnChunkRead struct {
	*Memory
	key, value string
	done       bool
}

func (s *rewriteOnChunkRead) Get(k string) (string, error) {
	if k != s.key && !s.done {
		s.done = true
		if err := (Chunked{Store: s.Memory, Size: 2}).Set(s.key, s.value); err != nil {
			return "", err
		}
	}
	return s.Memory.Get(k)
}

// TestChunkedGetDuringRewrite checks that a reader that loads the old head
// while a writer removes the old chunks gets the new value, not a miss.
func TestChunkedGetDuringRewrite(t *testing.T) {
	mem := &Memory{}
	if err := (Chunked{Store: mem, Size: 2}).Set("k", "abcd"); err != nil {
		t.Fatal(err)
	}
	c := Chunked{Store: &rewriteOnChunkRead{Memory: mem, key: "k", value: "wxyz"}, Size: 2}
	got, err := c.Get("k")
	if err != nil || got != "wxyz" {
		t.Fatalf("Get = %q, %v; want the new value", got, err)
	}
}

// TestChunkedConcurrentGetSet runs readers against a writer. A reader can
// see the old value or the new one, but never a miss.
func TestChunkedConcurrentGetSet(t *testing.T) {
	mem := &Memory{}
	c := Chunked{Store: mem, Size: 2}
	values := []string{"abcdef", "ghijklmn", "opq"}
	if err := c.Set("k", values[0]); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for range 4 {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				got, err := c.Get("k")
				if errors.Is(err, ErrNotFound) {
					t.Errorf("Get during Set = %v", err)
					return
				}
				if err == nil && !slices.Contains(values, got) {
					t.Errorf("Get = %q, a mix of values", got)
					return
				}
			}
		})
	}
	for i := range 2000 {
		if err := c.Set("k", values[i%len(values)]); err != nil {
			t.Fatal(err)
		}
	}
	close(stop)
	wg.Wait()
}

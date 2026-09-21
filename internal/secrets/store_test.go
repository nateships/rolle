package secrets

import (
	"errors"
	"strings"
	"testing"
)

// failStore returns err from every call.
type failStore struct{ err error }

func (f failStore) Get(string) (string, error) { return "", f.err }
func (f failStore) Set(string, string) error   { return f.err }
func (f failStore) Delete(string) error        { return f.err }

func TestMemoryStore(t *testing.T) {
	var m Memory
	if _, err := m.Get("k"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("zero store Get = %v, want ErrNotFound", err)
	}
	if err := m.Delete("k"); err != nil {
		t.Fatalf("delete on empty store: %v", err)
	}
	for _, v := range []string{"v1", "v2"} {
		if err := m.Set("k", v); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := m.Get("k"); err != nil || got != "v2" {
		t.Fatalf("Get = %q, %v; want v2", got, err)
	}
	// An empty value is a stored value, not a miss.
	if err := m.Set("k", ""); err != nil {
		t.Fatal(err)
	}
	if got, err := m.Get("k"); err != nil || got != "" {
		t.Fatalf("Get after empty Set = %q, %v", got, err)
	}
	if err := m.Delete("k"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Get("k"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after Delete = %v, want ErrNotFound", err)
	}
}

func TestChunkedSizeBoundary(t *testing.T) {
	cases := []struct {
		name   string
		value  string
		chunks int
	}{
		{"empty", "", 0},
		{"exactly size", "abcd", 0},
		{"one over", "abcde", 2},
		{"two exact", "abcdefgh", 2},
		{"two and a bit", "abcdefghi", 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mem := &Memory{}
			ch := Chunked{Store: mem, Size: 4}
			if err := ch.Set("k", c.value); err != nil {
				t.Fatal(err)
			}
			if got, err := ch.Get("k"); err != nil || got != c.value {
				t.Fatalf("Get = %q, %v", got, err)
			}
			head, err := mem.Get("k")
			if err != nil {
				t.Fatal(err)
			}
			if c.chunks == 0 {
				if head != c.value {
					t.Fatalf("small value stored as %q", head)
				}
			} else if !isChunked(head, c.chunks) {
				t.Fatalf("marker = %q, want chunks:%d", head, c.chunks)
			}
			if c.chunks > 0 {
				if _, err := mem.Get(chunkOf(t, mem, "k", c.chunks)); !errors.Is(err, ErrNotFound) {
					t.Fatalf("chunk %d exists past the end", c.chunks)
				}
			}
			if len(mem.m) != c.chunks+1 {
				t.Fatalf("%d entries stored, want %d", len(mem.m), c.chunks+1)
			}
		})
	}
}

func TestChunkedManyChunks(t *testing.T) {
	mem := &Memory{}
	ch := Chunked{Store: mem, Size: 3}
	value := strings.Repeat("0123456789", 30) // 300 bytes, 100 chunks
	if err := ch.Set("k", value); err != nil {
		t.Fatal(err)
	}
	if got, err := ch.Get("k"); err != nil || got != value {
		t.Fatalf("Get mismatch: %v", err)
	}
	if head, _ := mem.Get("k"); !isChunked(head, 100) {
		t.Fatalf("marker = %q", head)
	}
	if len(mem.m) != 101 {
		t.Fatalf("%d entries stored, want 101", len(mem.m))
	}
	if part, _ := mem.Get(chunkOf(t, mem, "k", 99)); part != "789" {
		t.Fatalf("last chunk = %q", part)
	}
}

func TestChunkedResizeLeavesNoStaleChunks(t *testing.T) {
	mem := &Memory{}
	ch := Chunked{Store: mem, Size: 2}
	if err := ch.Set("k", "abcdefgh"); err != nil { // 4 chunks
		t.Fatal(err)
	}
	var old []string
	for i := range 4 {
		old = append(old, chunkOf(t, mem, "k", i))
	}
	if err := ch.Set("k", "abcd"); err != nil { // 2 chunks
		t.Fatal(err)
	}
	if got, _ := ch.Get("k"); got != "abcd" {
		t.Fatalf("Get = %q", got)
	}
	for _, k := range old {
		if _, err := mem.Get(k); !errors.Is(err, ErrNotFound) {
			t.Fatalf("stale chunk %s remains", k)
		}
	}
	if len(mem.m) != 3 {
		t.Fatalf("%d entries after shrink, want 3", len(mem.m))
	}
	if err := ch.Set("k", "abcdef"); err != nil { // grow again to 3 chunks
		t.Fatal(err)
	}
	if got, _ := ch.Get("k"); got != "abcdef" || len(mem.m) != 4 {
		t.Fatalf("after grow: Get = %q, %d entries", got, len(mem.m))
	}
}

func TestChunkedGetErrors(t *testing.T) {
	mem := &Memory{}
	ch := Chunked{Store: mem, Size: 2}
	if err := mem.Set("bad", "chunks:x"); err != nil {
		t.Fatal(err)
	}
	if _, err := ch.Get("bad"); err == nil || !strings.Contains(err.Error(), "bad chunk marker") {
		t.Fatalf("bad marker: err = %v", err)
	}
	// A bad marker does not block a new Set.
	if err := ch.Set("bad", "v"); err != nil {
		t.Fatal(err)
	}
	if got, err := ch.Get("bad"); err != nil || got != "v" {
		t.Fatalf("after overwrite: %q, %v", got, err)
	}

	if err := ch.Set("k", "abcdef"); err != nil {
		t.Fatal(err)
	}
	if err := mem.Delete(chunkOf(t, mem, "k", 1)); err != nil {
		t.Fatal(err)
	}
	_, err := ch.Get("k")
	if !errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "chunk 1 of k") {
		t.Fatalf("missing chunk: err = %v", err)
	}
}

func TestChunkedDelete(t *testing.T) {
	mem := &Memory{}
	ch := Chunked{Store: mem, Size: 2}
	if err := ch.Delete("nope"); err != nil {
		t.Fatalf("delete of missing key: %v", err)
	}
	if err := ch.Set("p", "ab"); err != nil {
		t.Fatal(err)
	}
	if err := ch.Delete("p"); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Get("p"); !errors.Is(err, ErrNotFound) {
		t.Fatal("plain value not deleted")
	}
}

func TestChunkedPropagatesStoreErrors(t *testing.T) {
	boom := errors.New("boom")
	ch := Chunked{Store: failStore{boom}, Size: 2}
	if _, err := ch.Get("k"); !errors.Is(err, boom) {
		t.Fatalf("Get = %v", err)
	}
	if err := ch.Set("k", "abc"); !errors.Is(err, boom) {
		t.Fatalf("Set = %v", err)
	}
	if err := ch.Delete("k"); !errors.Is(err, boom) {
		t.Fatalf("Delete = %v", err)
	}
}

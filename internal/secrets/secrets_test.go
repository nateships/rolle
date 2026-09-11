package secrets

import (
	"strings"
	"testing"
)

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
	if head, _ := mem.Get("k"); head != "chunks:6" {
		t.Fatalf("marker = %q", head)
	}
	// A smaller value replaces the chunks and removes the leftovers.
	if err := c.Set("k", "small"); err != nil {
		t.Fatal(err)
	}
	if got, _ := c.Get("k"); got != "small" {
		t.Fatalf("got %q", got)
	}
	if _, err := mem.Get("k#0"); err != ErrNotFound {
		t.Fatal("stale chunk not removed")
	}
	if err := c.Set("k", big); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete("k"); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"k", "k#0", "k#5"} {
		if _, err := mem.Get(k); err != ErrNotFound {
			t.Fatalf("%s still present", k)
		}
	}
	if _, err := c.Get("missing"); err != ErrNotFound {
		t.Fatalf("missing key: %v", err)
	}
}

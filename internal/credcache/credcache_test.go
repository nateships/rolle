package credcache

import (
	"testing"
	"time"

	"github.com/nateships/rolle/internal/core"
)

func TestPutGetExpiry(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := &Cache{Dir: t.TempDir(), Now: func() time.Time { return now }}
	exp := now.Add(10 * time.Minute)
	if err := c.Put("s1", core.Credentials{AccessKeyID: "AKIA", Expiration: &exp}); err != nil {
		t.Fatal(err)
	}
	got, err := c.Get("s1")
	if err != nil || got.AccessKeyID != "AKIA" {
		t.Fatalf("got %+v, %v", got, err)
	}
	now = now.Add(6 * time.Minute)
	if _, err := c.Get("s1"); err != ErrMiss {
		t.Fatalf("expected miss inside skew, got %v", err)
	}
	if _, err := c.Get("missing"); err != ErrMiss {
		t.Fatalf("expected miss for unknown session, got %v", err)
	}
	if err := c.Delete("s1"); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete("s1"); err != nil {
		t.Fatalf("second delete: %v", err)
	}
}

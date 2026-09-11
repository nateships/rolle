package credcache

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nateships/rolle/internal/core"
)

var fixedNow = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func newCache(t *testing.T) *Cache {
	t.Helper()
	return &Cache{Dir: filepath.Join(t.TempDir(), "creds"), Now: func() time.Time { return fixedNow }}
}

func TestPutCreatesPrivateFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not meaningful on Windows")
	}
	c := newCache(t)
	if err := c.Put("s1", core.Credentials{AccessKeyID: "A"}); err != nil {
		t.Fatal(err)
	}
	dir, err := os.Stat(c.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := dir.Mode().Perm(); perm != 0o700 {
		t.Fatalf("dir perm = %o, want 700", perm)
	}
	file, err := os.Stat(c.path("s1"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := file.Mode().Perm(); perm != 0o600 {
		t.Fatalf("file perm = %o, want 600", perm)
	}
}

func TestPutLeavesOnlyTheCredentialFile(t *testing.T) {
	c := newCache(t)
	for _, id := range []string{"first", "second"} {
		if err := c.Put("s1", core.Credentials{AccessKeyID: id}); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(c.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "s1.json" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("dir contains %v, want only s1.json", names)
	}
	got, err := c.Get("s1")
	if err != nil || got.AccessKeyID != "second" {
		t.Fatalf("Get = %+v, %v", got, err)
	}
}

func TestPutRoundTripsAllFields(t *testing.T) {
	c := newCache(t)
	exp := fixedNow.Add(time.Hour)
	in := core.Credentials{AccessKeyID: "A", SecretAccessKey: "S", SessionToken: "T", Token: "bearer", Expiration: &exp}
	if err := c.Put("s1", in); err != nil {
		t.Fatal(err)
	}
	got, err := c.Get("s1")
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessKeyID != "A" || got.SecretAccessKey != "S" || got.SessionToken != "T" || got.Token != "bearer" {
		t.Fatalf("Get = %+v", got)
	}
	if got.Expiration == nil || !got.Expiration.Equal(exp) {
		t.Fatalf("Expiration = %v, want %v", got.Expiration, exp)
	}
}

func TestGetMisses(t *testing.T) {
	past := fixedNow.Add(-time.Minute)
	insideSkew := fixedNow.Add(4 * time.Minute)
	cases := []struct {
		name string
		data string
	}{
		{"corrupt json", "{not json"},
		{"empty file", ""},
		{"wrong type", `[]`},
		{"expired", `{"accessKeyId":"A","expiration":"` + past.Format(time.RFC3339) + `"}`},
		{"inside default skew", `{"accessKeyId":"A","expiration":"` + insideSkew.Format(time.RFC3339) + `"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newCache(t)
			if err := os.MkdirAll(c.Dir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(c.path("s1"), []byte(tc.data), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := c.Get("s1"); !errors.Is(err, ErrMiss) {
				t.Fatalf("Get = %v, want ErrMiss", err)
			}
		})
	}
}

func TestGetHonoursSkewAndMissingExpiry(t *testing.T) {
	c := newCache(t)
	exp := fixedNow.Add(2 * time.Minute)
	if err := c.Put("s1", core.Credentials{AccessKeyID: "A", Expiration: &exp}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get("s1"); !errors.Is(err, ErrMiss) {
		t.Fatalf("default skew of five minutes should miss, got %v", err)
	}
	c.Skew = time.Minute
	if got, err := c.Get("s1"); err != nil || got.AccessKeyID != "A" {
		t.Fatalf("one minute skew should hit, got %+v, %v", got, err)
	}
	if err := c.Put("forever", core.Credentials{Token: "t"}); err != nil {
		t.Fatal(err)
	}
	c.Now = func() time.Time { return fixedNow.Add(1000 * time.Hour) }
	if got, err := c.Get("forever"); err != nil || got.Token != "t" {
		t.Fatalf("credentials without expiry should always hit, got %+v, %v", got, err)
	}
}

func TestDeleteRemovesFile(t *testing.T) {
	c := newCache(t)
	if err := c.Put("s1", core.Credentials{AccessKeyID: "A"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete("s1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(c.path("s1")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file still present: %v", err)
	}
	if _, err := c.Get("s1"); !errors.Is(err, ErrMiss) {
		t.Fatalf("Get after Delete = %v", err)
	}
}

func TestDefaultHonoursCacheDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ROLLE_CACHE_DIR", dir)
	if got := Default().Dir; got != dir {
		t.Fatalf("Dir = %q, want %q", got, dir)
	}
	t.Setenv("ROLLE_CACHE_DIR", "")
	if got := Default().Dir; !strings.Contains(got, "rolle") || !strings.HasSuffix(got, "credentials") {
		t.Fatalf("Dir = %q, want a rolle credentials directory", got)
	}
}

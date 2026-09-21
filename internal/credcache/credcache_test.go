package credcache

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/secrets"
)

var fixedNow = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func newCache(t *testing.T) *Cache {
	t.Helper()
	return &Cache{Store: &secrets.Memory{}, Dir: filepath.Join(t.TempDir(), "creds"), Now: func() time.Time { return fixedNow }}
}

func TestPutGetExpiry(t *testing.T) {
	now := fixedNow
	c := newCache(t)
	c.Now = func() time.Time { return now }
	exp := now.Add(10 * time.Minute)
	if err := c.Put("s1", core.Credentials{AccessKeyID: "AKIA", Expiration: &exp}); err != nil {
		t.Fatal(err)
	}
	got, err := c.Get("s1")
	if err != nil || got.AccessKeyID != "AKIA" {
		t.Fatalf("got %+v, %v", got, err)
	}
	now = now.Add(6 * time.Minute)
	if _, err := c.Get("s1"); !errors.Is(err, ErrMiss) {
		t.Fatalf("expected miss inside skew, got %v", err)
	}
	if _, err := c.Get("missing"); !errors.Is(err, ErrMiss) {
		t.Fatalf("expected miss for unknown session, got %v", err)
	}
	if err := c.Delete("s1"); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete("s1"); err != nil {
		t.Fatalf("second delete: %v", err)
	}
}

func TestPutWritesNoFile(t *testing.T) {
	c := newCache(t)
	if err := c.Put("s1", core.Credentials{AccessKeyID: "A", SecretAccessKey: "S"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(c.Dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Put must not touch the directory: %v", err)
	}
	raw, err := c.Store.Get("credentials/s1")
	if err != nil || !strings.Contains(raw, `"accessKeyId":"A"`) {
		t.Fatalf("stored = %q, %v", raw, err)
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
		{"empty value", ""},
		{"wrong type", `[]`},
		{"expired", `{"accessKeyId":"A","expiration":"` + past.Format(time.RFC3339) + `"}`},
		{"inside default skew", `{"accessKeyId":"A","expiration":"` + insideSkew.Format(time.RFC3339) + `"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newCache(t)
			if err := c.Store.Set("credentials/s1", tc.data); err != nil {
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
	c.Skew = time.Minute
	if got, err := c.Get("s1"); err != nil || got.AccessKeyID != "A" {
		t.Fatalf("one minute skew should hit, got %+v, %v", got, err)
	}
	c.Skew = 0
	if _, err := c.Get("s1"); !errors.Is(err, ErrMiss) {
		t.Fatalf("default skew of five minutes should miss, got %v", err)
	}
	if err := c.Put("forever", core.Credentials{Token: "t"}); err != nil {
		t.Fatal(err)
	}
	c.Now = func() time.Time { return fixedNow.Add(1000 * time.Hour) }
	if got, err := c.Get("forever"); err != nil || got.Token != "t" {
		t.Fatalf("credentials without expiry should always hit, got %+v, %v", got, err)
	}
}

func TestDeleteRemovesEntry(t *testing.T) {
	c := newCache(t)
	if err := c.Put("s1", core.Credentials{AccessKeyID: "A"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete("s1"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Store.Get("credentials/s1"); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("entry still present: %v", err)
	}
	if _, err := c.Get("s1"); !errors.Is(err, ErrMiss) {
		t.Fatalf("Get after Delete = %v", err)
	}
}

func TestDeleteToleratesAFailingStore(t *testing.T) {
	c := &Cache{Store: failStore{secrets.ErrNotFound}}
	if err := c.Delete("s1"); err != nil {
		t.Fatalf("Delete of a missing entry = %v", err)
	}
	locked := errors.New("keychain locked")
	c.Store = failStore{locked}
	if err := c.Delete("s1"); err == nil {
		t.Fatal("Delete must report a store fault")
	}
	// A store fault is not a miss: a miss would send the caller to the provider.
	if _, err := c.Get("s1"); !errors.Is(err, locked) || errors.Is(err, ErrMiss) {
		t.Fatalf("Get with a failing store = %v", err)
	}
}

func TestMargin(t *testing.T) {
	c := &Cache{}
	if c.Margin() != 5*time.Minute {
		t.Fatalf("default margin = %v", c.Margin())
	}
	c.Skew = time.Second
	if c.Margin() != time.Second {
		t.Fatalf("margin = %v", c.Margin())
	}
}

// failStore returns err from every call.
type failStore struct{ err error }

func (f failStore) Get(string) (string, error) { return "", f.err }
func (f failStore) Set(string, string) error   { return f.err }
func (f failStore) Delete(string) error        { return f.err }

func TestDefaultHonoursCacheDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ROLLE_CACHE_DIR", dir)
	// A credential file from a version before the keychain cache goes; the
	// files other tools read stay.
	if err := os.MkdirAll(filepath.Join(dir, "gcp"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"s1.json", filepath.Join("gcp", "g1.json")} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	c := Default()
	if c.Dir != dir || c.Store == nil {
		t.Fatalf("Default = %+v", c)
	}
	if _, err := os.Stat(filepath.Join(dir, "s1.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy credential file still present: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "gcp", "g1.json")); err != nil {
		t.Fatalf("gcp file removed: %v", err)
	}
	t.Setenv("ROLLE_CACHE_DIR", "")
	if got := Default().Dir; !strings.Contains(got, "rolle") || !strings.HasSuffix(got, "credentials") {
		t.Fatalf("Dir = %q, want a rolle credentials directory", got)
	}
}

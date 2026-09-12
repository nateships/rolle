package azure

import (
	"context"
	"errors"
	"testing"

	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/secrets"
)

func TestTokenWithoutLoginIsLoginRequired(t *testing.T) {
	a := &Auth{Integration: core.Integration{ID: "i1", Azure: &core.AzureIntegration{TenantID: "t", Account: "me@contoso.com"}}, Secrets: &secrets.Memory{}}
	_, err := a.Token(context.Background())
	if !errors.Is(err, ErrLoginRequired) {
		t.Fatalf("Token = %v, want ErrLoginRequired", err)
	}
}

func TestLogoutReportsStoreFailure(t *testing.T) {
	boom := errors.New("boom")
	a := &Auth{Integration: core.Integration{ID: "i1"}, Secrets: failStore{boom}}
	if err := a.Logout(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("Logout = %v, want the store error", err)
	}
	// A broken store must not look like a signed-out user.
	if _, err := a.Token(context.Background()); err == nil || errors.Is(err, ErrLoginRequired) {
		t.Fatalf("Token with a broken store = %v", err)
	}
}

func TestLogoutClearsTheCacheEntry(t *testing.T) {
	mem := &secrets.Memory{}
	if err := mem.Set(cacheKey("i1"), "{}"); err != nil {
		t.Fatal(err)
	}
	a := &Auth{Integration: core.Integration{ID: "i1"}, Secrets: mem}
	if err := a.Logout(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Get(cacheKey("i1")); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatal("MSAL cache entry not removed")
	}
}

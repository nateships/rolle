package aws

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/secrets"
)

var ssoNow = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func newSSO(store secrets.Store, now *time.Time) *SSO {
	return &SSO{
		Integration: core.Integration{ID: "i1", AWSSSO: &core.AWSSSOIntegration{StartURL: "https://acme.awsapps.com/start", Region: "us-east-1"}},
		Secrets:     store,
		Now:         func() time.Time { return *now },
	}
}

func TestSSOTokenLifecycle(t *testing.T) {
	now := ssoNow
	mem := &secrets.Memory{}
	s := newSSO(mem, &now)
	ctx := context.Background()

	if exp := s.TokenExpiry(); exp != nil {
		t.Fatalf("TokenExpiry before login = %v", exp)
	}
	if _, err := s.token(); !errors.Is(err, ErrSSOLoginRequired) {
		t.Fatalf("token before login = %v", err)
	}
	// Every portal call fails before touching the network when no token exists.
	if _, err := s.ListAccounts(ctx); !errors.Is(err, ErrSSOLoginRequired) {
		t.Fatalf("ListAccounts = %v", err)
	}
	if _, err := s.ListRoles(ctx, "1"); !errors.Is(err, ErrSSOLoginRequired) {
		t.Fatalf("ListRoles = %v", err)
	}
	if _, err := s.RoleCredentials(ctx, "1", "Admin"); !errors.Is(err, ErrSSOLoginRequired) {
		t.Fatalf("RoleCredentials = %v", err)
	}

	expires := now.Add(time.Hour)
	if err := s.StoreImportedToken("at", "rt", "cid", "cs", "", expires); err != nil {
		t.Fatal(err)
	}
	raw, err := mem.Get("aws-sso-token/i1")
	if err != nil {
		t.Fatalf("token stored under an unexpected key: %v", err)
	}
	var stored ssoToken
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.AccessToken != "at" || stored.RefreshToken != "rt" || stored.ClientID != "cid" || stored.ClientSecret != "cs" {
		t.Fatalf("stored = %+v", stored)
	}
	if stored.Region != "us-east-1" {
		t.Fatalf("region = %q, want the integration region", stored.Region)
	}
	if !stored.Expires.Equal(expires) {
		t.Fatalf("expires = %v, want %v", stored.Expires, expires)
	}
	if got := s.TokenExpiry(); got == nil || !got.Equal(expires) {
		t.Fatalf("TokenExpiry = %v, want %v", got, expires)
	}
	tok, err := s.token()
	if err != nil || tok.AccessToken != "at" {
		t.Fatalf("token = %+v, %v", tok, err)
	}

	// The token counts as expired one minute early.
	now = expires.Add(-90 * time.Second)
	if s.TokenExpiry() == nil {
		t.Fatal("token 90s before expiry should still be valid")
	}
	now = expires.Add(-30 * time.Second)
	if s.TokenExpiry() != nil {
		t.Fatal("token 30s before expiry should count as expired")
	}
	if _, err := s.token(); !errors.Is(err, ErrSSOLoginRequired) {
		t.Fatalf("expired token = %v", err)
	}

	if err := s.Logout(); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Get("aws-sso-token/i1"); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatal("Logout left the token in the store")
	}
	if err := s.Logout(); err != nil {
		t.Fatalf("second Logout: %v", err)
	}
}

func TestStoreImportedTokenKeepsExplicitRegion(t *testing.T) {
	now := ssoNow
	mem := &secrets.Memory{}
	s := newSSO(mem, &now)
	loc := time.FixedZone("plus2", 2*3600)
	if err := s.StoreImportedToken("at", "", "cid", "cs", "eu-west-1", now.In(loc).Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	raw, _ := mem.Get("aws-sso-token/i1")
	var stored ssoToken
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Region != "eu-west-1" || stored.Expires.Location() != time.UTC || !stored.Expires.Equal(now.Add(time.Hour)) {
		t.Fatalf("stored = %+v", stored)
	}
}

func TestSSOTokenErrors(t *testing.T) {
	now := ssoNow
	mem := &secrets.Memory{}
	s := newSSO(mem, &now)
	if err := mem.Set("aws-sso-token/i1", "not json"); err != nil {
		t.Fatal(err)
	}
	if s.TokenExpiry() != nil {
		t.Fatal("corrupt token reported an expiry")
	}
	if _, err := s.token(); err == nil || errors.Is(err, ErrSSOLoginRequired) {
		t.Fatalf("corrupt token = %v, want a parse error", err)
	}

	boom := errors.New("boom")
	failing := newSSO(failStore{boom}, &now)
	if _, err := failing.token(); !errors.Is(err, boom) {
		t.Fatalf("store error = %v", err)
	}
	if err := failing.StoreImportedToken("a", "", "c", "s", "", now); !errors.Is(err, boom) {
		t.Fatalf("store error on Set = %v", err)
	}
}

func TestSSONowDefaultsToWallClock(t *testing.T) {
	s := &SSO{}
	before := time.Now()
	got := s.now()
	if got.Before(before) || got.After(time.Now()) {
		t.Fatalf("now() = %v", got)
	}
}

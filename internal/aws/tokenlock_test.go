package aws

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nateships/rolle/internal/filelock"
	"github.com/nateships/rolle/internal/secrets"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestLoginRefreshRunsOnce starts two renewals of one lapsed sign-in at the
// same time, as the desktop app and a credential_process call can. The lock
// lets one refresh run. The other reads the stored result and does not spend
// the rotated refresh token again.
func TestLoginRefreshRunsOnce(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pemKey, err := marshalKey(key)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		// Keep the refresh in flight long enough for the second caller to read
		// the lapsed token.
		time.Sleep(100 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(tokenBody("AKIA2", "rt2", ""))
	}))
	t.Cleanup(srv.Close)
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		r = r.Clone(r.Context())
		r.URL.Scheme, r.URL.Host = target.Scheme, target.Host
		return http.DefaultTransport.RoundTrip(r)
	})}

	now := ssoNow
	store := &secrets.Memory{}
	dir := t.TempDir()
	newLogin := func() *Login {
		return &Login{SessionID: "s1", Region: "us-east-1", Secrets: store, Now: func() time.Time { return now }, Client: client, LockDir: dir}
	}
	if err := newLogin().store(loginToken{
		AccessKeyID: "AKIA1", Expires: now.Add(-time.Minute), RefreshToken: "rt1", DPoPKey: pemKey, Region: "us-east-1",
	}); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			<-start
			creds, err := newLogin().Credentials(context.Background())
			if err != nil || creds.AccessKeyID != "AKIA2" {
				t.Errorf("Credentials = %+v, %v", creds, err)
			}
		})
	}
	close(start)
	wg.Wait()
	if n := calls.Load(); n != 1 {
		t.Fatalf("refresh ran %d times, want 1", n)
	}
}

// TestLoginRefreshHoldsLockForLimitedTime checks that a refresh that hangs
// stops and releases the lock. Then it cannot block the next process.
func TestLoginRefreshHoldsLockForLimitedTime(t *testing.T) {
	old := lockHold
	lockHold = 100 * time.Millisecond
	t.Cleanup(func() { lockHold = old })
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pemKey, err := marshalKey(key)
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	client, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	t.Cleanup(func() { close(release) })
	now := ssoNow
	l := &Login{SessionID: "s1", Region: "us-east-1", Secrets: &secrets.Memory{}, Now: func() time.Time { return now }, Client: client, LockDir: t.TempDir()}
	if err := l.store(loginToken{
		AccessKeyID: "AKIA1", Expires: now.Add(-time.Minute), RefreshToken: "rt1", DPoPKey: pemKey, Region: "us-east-1",
	}); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := l.Credentials(context.Background())
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || errors.Is(err, ErrLoginRequired) {
			t.Fatalf("hung refresh = %v, want a transient error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("hung refresh still holds the lock")
	}
	unlock, err := filelock.Lock(context.Background(), tokenLockPath(l.LockDir, loginTokenKey(l.SessionID)))
	if err != nil {
		t.Fatal(err)
	}
	unlock()
}

// TestSSOTokenReadsAgainUnderLock holds the lock while another process stores
// a renewed token. The waiting caller must use that token and not refresh.
func TestSSOTokenReadsAgainUnderLock(t *testing.T) {
	now := ssoNow
	mem := &secrets.Memory{}
	s := newSSO(mem, &now)
	s.LockDir = t.TempDir()
	if err := s.storeToken(ssoToken{
		AccessToken: "old", RefreshToken: "rt1", ClientID: "c", ClientSecret: "cs", Expires: now.Add(-time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	unlock, err := filelock.Lock(context.Background(), tokenLockPath(s.LockDir, ssoTokenKey(s.Integration.ID)))
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		tok ssoToken
		err error
	}
	got := make(chan result, 1)
	go func() {
		tok, err := s.token(context.Background())
		got <- result{tok, err}
	}()
	// The other process renews the token while it holds the lock.
	time.Sleep(100 * time.Millisecond)
	if err := s.storeToken(ssoToken{
		AccessToken: "new", RefreshToken: "rt2", ClientID: "c", ClientSecret: "cs", Expires: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	unlock()
	select {
	case r := <-got:
		if r.err != nil || r.tok.AccessToken != "new" {
			t.Fatalf("token = %+v, %v; want the token stored under the lock", r.tok, r.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("token did not return after unlock")
	}
}

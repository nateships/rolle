package aws

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nateships/rolle/internal/secrets"
)

// TestStartLoginPKCE drives the browser flow against a fake OIDC service: the
// registration asks for the loopback redirect, the authorization page carries
// a S256 challenge, the callback code is exchanged with the matching verifier,
// and the token lands in the secret store.
func TestStartLoginPKCE(t *testing.T) {
	var registered struct {
		GrantTypes   []string `json:"grantTypes"`
		RedirectUris []string `json:"redirectUris"`
		Scopes       []string `json:"scopes"`
	}
	var tokenReq map[string]any
	oidc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/client/register":
			_ = json.NewDecoder(r.Body).Decode(&registered)
			_ = json.NewEncoder(w).Encode(map[string]any{"clientId": "cid", "clientSecret": "csec", "clientIdIssuedAt": 1, "clientSecretExpiresAt": 9999999999})
		case "/token":
			_ = json.NewDecoder(r.Body).Decode(&tokenReq)
			_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "at", "refreshToken": "rt", "expiresIn": 3600, "tokenType": "Bearer"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer oidc.Close()
	t.Setenv("AWS_ENDPOINT_URL_SSO_OIDC", oidc.URL)

	store := &secrets.Memory{}
	now := time.Now()
	s := newSSO(store, &now)
	auth, err := s.StartLogin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if auth.UserCode != "" {
		t.Fatalf("browser flow has no user code, got %q", auth.UserCode)
	}
	if strings.Join(registered.GrantTypes, ",") != "authorization_code,refresh_token" || strings.Join(registered.Scopes, ",") != scopeAccountAccess {
		t.Fatalf("registration: %+v", registered)
	}
	page, err := url.Parse(auth.VerificationURI)
	if err != nil {
		t.Fatal(err)
	}
	q := page.Query()
	if page.Host != "oidc.us-east-1.amazonaws.com" || q.Get("response_type") != "code" || q.Get("code_challenge_method") != "S256" || q.Get("scopes") != scopeAccountAccess {
		t.Fatalf("authorize URL: %s", auth.VerificationURI)
	}
	redirect := q.Get("redirect_uri")
	if len(registered.RedirectUris) != 1 || registered.RedirectUris[0] != redirect || !strings.HasPrefix(redirect, "http://127.0.0.1:") {
		t.Fatalf("redirect: %q vs %v", redirect, registered.RedirectUris)
	}

	done := make(chan error, 1)
	go func() { done <- auth.Wait(context.Background()) }()
	resp, err := http.Get(redirect + "?code=abc&state=" + url.QueryEscape(q.Get("state")))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("callback status %d", resp.StatusCode)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Wait did not return")
	}

	if tokenReq["grantType"] != "authorization_code" || tokenReq["code"] != "abc" || tokenReq["redirectUri"] != redirect {
		t.Fatalf("token request: %v", tokenReq)
	}
	sum := sha256.Sum256([]byte(tokenReq["codeVerifier"].(string)))
	if base64.RawURLEncoding.EncodeToString(sum[:]) != q.Get("code_challenge") {
		t.Fatal("code verifier does not match the challenge")
	}
	tok, err := s.token(context.Background())
	if err != nil || tok.AccessToken != "at" || tok.RefreshToken != "rt" || tok.ClientID != "cid" {
		t.Fatalf("stored token %+v err %v", tok, err)
	}
}

// TestStartLoginPKCERejectsWrongState makes sure a callback for another login
// does not complete this one.
func TestStartLoginPKCERejectsWrongState(t *testing.T) {
	oidc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"clientId": "cid", "clientSecret": "csec", "clientIdIssuedAt": 1, "clientSecretExpiresAt": 9999999999})
	}))
	defer oidc.Close()
	t.Setenv("AWS_ENDPOINT_URL_SSO_OIDC", oidc.URL)
	now := time.Now()
	auth, err := newSSO(&secrets.Memory{}, &now).StartLogin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	q := must(url.Parse(auth.VerificationURI)).Query()
	redirect := q.Get("redirect_uri")
	done := make(chan error, 1)
	go func() { done <- auth.Wait(context.Background()) }()
	// A request with another state is refused and does not end the wait.
	resp, err := http.Get(redirect + "?code=abc&state=other")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("foreign callback status %d", resp.StatusCode)
	}
	select {
	case err := <-done:
		t.Fatalf("Wait returned %v after a foreign callback", err)
	case <-time.After(200 * time.Millisecond):
	}
	// The real redirect still completes the flow.
	resp, err = http.Get(redirect + "?error=access_denied&state=" + url.QueryEscape(q.Get("state")))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if err := <-done; err == nil || !strings.Contains(err.Error(), "access_denied") {
		t.Fatalf("err = %v", err)
	}
}

// TestCancelFreesListener makes sure an abandoned browser login closes its port.
func TestCancelFreesListener(t *testing.T) {
	oidc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"clientId": "cid", "clientSecret": "csec", "clientIdIssuedAt": 1, "clientSecretExpiresAt": 9999999999})
	}))
	defer oidc.Close()
	t.Setenv("AWS_ENDPOINT_URL_SSO_OIDC", oidc.URL)
	now := time.Now()
	auth, err := newSSO(&secrets.Memory{}, &now).StartLogin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	redirect := must(url.Parse(auth.VerificationURI)).Query().Get("redirect_uri")
	auth.Cancel()
	if _, err := http.Get(redirect); err == nil {
		t.Fatal("listener still accepts connections after Cancel")
	}
}

// TestTokenRefresh trades an expired access token for a new one with the
// stored refresh token instead of asking for a new login.
func TestTokenRefresh(t *testing.T) {
	var tokenReq map[string]any
	oidc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&tokenReq)
		_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "at2", "refreshToken": "rt2", "expiresIn": 3600, "tokenType": "Bearer"})
	}))
	defer oidc.Close()
	t.Setenv("AWS_ENDPOINT_URL_SSO_OIDC", oidc.URL)

	now := ssoNow
	store := &secrets.Memory{}
	s := newSSO(store, &now)
	if err := s.StoreImportedToken("at", "rt", "cid", "cs", "", now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	tok, err := s.token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tokenReq["grantType"] != "refresh_token" || tokenReq["refreshToken"] != "rt" || tokenReq["clientId"] != "cid" || tokenReq["clientSecret"] != "cs" {
		t.Fatalf("token request: %v", tokenReq)
	}
	if tok.AccessToken != "at2" || tok.RefreshToken != "rt2" || !tok.Expires.Equal(now.Add(time.Hour)) {
		t.Fatalf("token = %+v", tok)
	}
	if exp := s.TokenExpiry(); exp == nil || !exp.Equal(now.Add(time.Hour)) {
		t.Fatalf("TokenExpiry after refresh = %v", exp)
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func TestAuthorizeURLChinaPartition(t *testing.T) {
	u := authorizeURL("cn-north-1", "c", "http://127.0.0.1:1/x", "s", "ch")
	if !strings.HasPrefix(u, "https://oidc.cn-north-1.amazonaws.com.cn/authorize?") {
		t.Fatal(u)
	}
}

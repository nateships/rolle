package aws

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nateships/rolle/internal/secrets"
)

func TestLoginAuthorizeURL(t *testing.T) {
	u, err := url.Parse(loginAuthorizeURL("eu-west-1", "http://127.0.0.1:1/oauth/callback", "st", "ch"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if u.Host != "eu-west-1.oauth.signin.aws" || u.Path != "/v1/authorize" {
		t.Fatalf("url = %s", u)
	}
	want := map[string]string{
		"response_type": "code", "client_id": loginClientID, "redirect_uri": "http://127.0.0.1:1/oauth/callback",
		"state": "st", "code_challenge": "ch", "code_challenge_method": "SHA-256", "scope": "openid",
	}
	for k, v := range want {
		if q.Get(k) != v {
			t.Errorf("%s = %q, want %q", k, q.Get(k), v)
		}
	}
	for region, host := range map[string]string{
		"us-gov-west-1": "us-gov-west-1.signin.amazonaws-us-gov.com",
		"cn-north-1":    "cn-north-1.signin.amazonaws.cn",
	} {
		if got := loginHost(region); got != host {
			t.Errorf("loginHost(%s) = %s, want %s", region, got, host)
		}
	}
}

// verifyDPoP checks a proof against the key in its own header and returns the claims.
func verifyDPoP(t *testing.T, proof, uri string) map[string]any {
	t.Helper()
	parts := strings.Split(proof, ".")
	if len(parts) != 3 {
		t.Fatalf("proof has %d parts", len(parts))
	}
	dec := func(s string) []byte {
		b, err := base64.RawURLEncoding.DecodeString(s)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	var header struct {
		Typ string `json:"typ"`
		Alg string `json:"alg"`
		JWK struct{ Kty, Crv, X, Y string }
	}
	if err := json.Unmarshal(dec(parts[0]), &header); err != nil {
		t.Fatal(err)
	}
	if header.Typ != "dpop+jwt" || header.Alg != "ES256" || header.JWK.Kty != "EC" || header.JWK.Crv != "P-256" {
		t.Fatalf("header = %+v", header)
	}
	point := append(append([]byte{4}, dec(header.JWK.X)...), dec(header.JWK.Y)...)
	pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), point)
	if err != nil {
		t.Fatal(err)
	}
	sig := dec(parts[2])
	if len(sig) != 64 {
		t.Fatalf("signature is %d bytes", len(sig))
	}
	der, err := asn1.Marshal(struct{ R, S *big.Int }{new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])})
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.VerifyASN1(pub, sum[:], der) {
		t.Fatal("signature does not verify")
	}
	var claims map[string]any
	if err := json.Unmarshal(dec(parts[1]), &claims); err != nil {
		t.Fatal(err)
	}
	if claims["htm"] != "POST" || claims["htu"] != uri || claims["jti"] == "" {
		t.Fatalf("claims = %v", claims)
	}
	return claims
}

func TestDPoPProof(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	proof, err := dpopProof(key, "https://us-east-1.oauth.signin.aws/v1/token", now)
	if err != nil {
		t.Fatal(err)
	}
	claims := verifyDPoP(t, proof, "https://us-east-1.oauth.signin.aws/v1/token")
	if iat, _ := claims["iat"].(float64); int64(iat) != now.Unix() {
		t.Fatalf("iat = %v", claims["iat"])
	}
	// The key survives the PEM round trip that the secret store needs.
	pemKey, err := marshalKey(key)
	if err != nil {
		t.Fatal(err)
	}
	back, err := parseKey(pemKey)
	if err != nil {
		t.Fatal(err)
	}
	if !back.Equal(key) {
		t.Fatal("key changed across PEM round trip")
	}
}

// fakeIDToken is an unsigned JWT whose subject is a sign-in session ARN.
func fakeIDToken(sub string) string {
	enc := func(v any) string {
		b, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(b)
	}
	return enc(map[string]string{"alg": "none"}) + "." + enc(map[string]string{"sub": sub}) + ".sig"
}

func tokenBody(id, refresh, idToken string) map[string]any {
	return map[string]any{
		"accessToken":  map[string]string{"accessKeyId": id, "secretAccessKey": "secret-" + id, "sessionToken": "tok-" + id},
		"tokenType":    "aws_sigv4",
		"expiresIn":    900,
		"refreshToken": refresh,
		"idToken":      idToken,
	}
}

// TestLoginFlow drives the browser flow against a fake token endpoint: the
// callback code is exchanged with the matching verifier and a valid DPoP
// proof, and the credentials land in the secret store with the session ARN.
func TestLoginFlow(t *testing.T) {
	const session = "arn:aws:signin:us-east-1:123456789012:session/abc"
	var got map[string]string
	var proof string
	client, rt := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/token" || r.Header.Get("Content-Type") != "application/json" {
			http.NotFound(w, r)
			return
		}
		proof = r.Header.Get("DPoP")
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(tokenBody("AKIA1", "rt1", fakeIDToken(session)))
	})
	store := &secrets.Memory{}
	now := ssoNow
	l := &Login{SessionID: "s1", Region: "us-east-1", Secrets: store, Now: func() time.Time { return now }, Client: client}

	if _, err := l.Credentials(context.Background()); !errors.Is(err, ErrLoginRequired) {
		t.Fatalf("Credentials before login = %v", err)
	}
	auth, err := l.StartLogin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	page, err := url.Parse(auth.VerificationURI)
	if err != nil {
		t.Fatal(err)
	}
	q := page.Query()
	redirect := q.Get("redirect_uri")
	if page.Host != "us-east-1.oauth.signin.aws" || !strings.HasPrefix(redirect, "http://127.0.0.1:") || !strings.HasSuffix(redirect, redirectPath) {
		t.Fatalf("authorize URL: %s", auth.VerificationURI)
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
	case <-time.After(5 * time.Second):
		t.Fatal("Wait did not return")
	}
	if rt.calls != 1 {
		t.Fatalf("token endpoint called %d times", rt.calls)
	}
	if got["grantType"] != "authorization_code" || got["code"] != "abc" || got["clientId"] != loginClientID || got["redirectUri"] != redirect {
		t.Fatalf("token request: %v", got)
	}
	if pkceChallenge(got["codeVerifier"]) != q.Get("code_challenge") {
		t.Fatal("code verifier does not match the challenge")
	}
	verifyDPoP(t, proof, "https://us-east-1.oauth.signin.aws/v1/token")

	if l.LoginSession() != session {
		t.Fatalf("LoginSession = %q", l.LoginSession())
	}
	if AccountFromLoginSession(session) != "123456789012" {
		t.Fatalf("account = %q", AccountFromLoginSession(session))
	}
	creds, err := l.Credentials(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if creds.AccessKeyID != "AKIA1" || creds.SessionToken != "tok-AKIA1" || creds.Expiration == nil || !creds.Expiration.Equal(now.Add(15*time.Minute)) {
		t.Fatalf("creds = %+v", creds)
	}
	if rt.calls != 1 {
		t.Fatal("valid credentials should be served without the network")
	}
}

func TestLoginRefresh(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pemKey, err := marshalKey(key)
	if err != nil {
		t.Fatal(err)
	}
	now := ssoNow
	var got map[string]string
	var status int
	var errCode string
	client, rt := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		if status != 0 {
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": errCode, "message": "nope"})
			return
		}
		_ = json.NewEncoder(w).Encode(tokenBody("AKIA2", "rt2", ""))
	})
	store := &secrets.Memory{}
	l := &Login{SessionID: "s1", Region: "us-east-1", Secrets: store, Now: func() time.Time { return now }, Client: client}
	seed := func() {
		if err := l.store(loginToken{
			AccessKeyID: "AKIA1", SecretAccessKey: "s", SessionToken: "t", Expires: now.Add(-time.Minute),
			RefreshToken: "rt1", DPoPKey: pemKey, Region: "us-east-1", LoginSession: "arn:aws:signin:us-east-1:1:session/x",
		}); err != nil {
			t.Fatal(err)
		}
	}

	seed()
	creds, err := l.Credentials(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got["grantType"] != "refresh_token" || got["refreshToken"] != "rt1" || got["clientId"] != loginClientID {
		t.Fatalf("refresh request: %v", got)
	}
	if creds.AccessKeyID != "AKIA2" || !creds.Expiration.Equal(now.Add(15*time.Minute)) {
		t.Fatalf("creds = %+v", creds)
	}
	stored, err := l.stored()
	if err != nil {
		t.Fatal(err)
	}
	if stored.RefreshToken != "rt2" || stored.LoginSession != "arn:aws:signin:us-east-1:1:session/x" || stored.DPoPKey != pemKey {
		t.Fatalf("stored = %+v", stored)
	}
	if rt.calls != 1 {
		t.Fatalf("calls = %d", rt.calls)
	}

	// A refused refresh token needs a new sign-in.
	seed()
	status, errCode = http.StatusUnauthorized, "TOKEN_EXPIRED"
	if _, err := l.Credentials(context.Background()); !errors.Is(err, ErrLoginRequired) {
		t.Fatalf("expired refresh token: %v", err)
	}
	// A service fault does not.
	status, errCode = http.StatusInternalServerError, "INTERNAL"
	if _, err := l.Credentials(context.Background()); err == nil || errors.Is(err, ErrLoginRequired) {
		t.Fatalf("service fault: %v", err)
	}
	// So does a sign-in without a refresh token.
	status = 0
	if err := l.store(loginToken{AccessKeyID: "AKIA1", Expires: now.Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Credentials(context.Background()); !errors.Is(err, ErrLoginRequired) {
		t.Fatalf("no refresh token: %v", err)
	}
	if err := l.Logout(); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Credentials(context.Background()); !errors.Is(err, ErrLoginRequired) {
		t.Fatalf("after logout: %v", err)
	}
}

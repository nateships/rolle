package aws

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/netcfg"
	"github.com/nateships/rolle/internal/secrets"
)

// The console sign-in flow is the one behind `aws login`: an authorization
// code grant with PKCE against AWS Sign-In, a DPoP-bound refresh token, and
// an access token that is a set of short-lived SigV4 credentials. The
// reference is the AWS CLI v2 source, awscli/customizations/login, and the
// signin service model shipped with it.
const (
	// loginClientID is the public client the same-device flow uses.
	loginClientID = "arn:aws:signin:::devtools/same-device"
	loginScope    = "openid"
)

// loginToken is the cached sign-in of one console login session.
type loginToken struct {
	AccessKeyID     string    `json:"accessKeyId"`
	SecretAccessKey string    `json:"secretAccessKey"`
	SessionToken    string    `json:"sessionToken"`
	Expires         time.Time `json:"expires"`
	RefreshToken    string    `json:"refreshToken"`
	// DPoPKey is the PEM EC private key the refresh token is bound to.
	DPoPKey string `json:"dpopKey"`
	Region  string `json:"region"`
	// LoginSession is the sign-in session ARN, from the id token.
	LoginSession string `json:"loginSession"`
}

func loginTokenKey(sessionID string) string { return "aws-login-token/" + sessionID }

// ErrLoginRequired is returned when no usable console sign-in exists.
var ErrLoginRequired = errors.New("aws login: login required")

// Login authenticates one session with console credentials.
type Login struct {
	SessionID string
	Region    string
	Secrets   secrets.Store
	// Now is overridable for tests.
	Now func() time.Time
	// Client is overridable for tests. Nil uses the configured client.
	Client *http.Client
}

func (l *Login) now() time.Time {
	if l.Now != nil {
		return l.Now()
	}
	return time.Now()
}

func (l *Login) client() *http.Client {
	if l.Client != nil {
		return l.Client
	}
	return netcfg.Client()
}

// loginHost is the AWS Sign-In OAuth host for a region.
func loginHost(region string) string {
	switch {
	case strings.HasPrefix(region, "us-gov-"):
		return region + ".signin.amazonaws-us-gov.com"
	case strings.HasPrefix(region, "cn-"):
		return region + ".signin.amazonaws.cn"
	}
	return region + ".oauth.signin.aws"
}

// loginAuthorizeURL builds the sign-in page for the PKCE flow.
func loginAuthorizeURL(region, redirect, state, challenge string) string {
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {loginClientID},
		"redirect_uri":          {redirect},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"SHA-256"},
		"scope":                 {loginScope},
	}
	return "https://" + loginHost(region) + "/v1/authorize?" + q.Encode()
}

// StartLogin begins a browser sign-in. The browser returns to a loopback
// listener; Wait then trades the code for credentials and stores them.
func (l *Login) StartLogin(ctx context.Context) (*DeviceAuthorization, error) {
	state := randomToken(16)
	lb, err := listenLoopback(state)
	if err != nil {
		return nil, fmt.Errorf("aws login: %w", err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		lb.stop()
		return nil, err
	}
	verifier := randomToken(32)
	redirect := lb.redirect
	d := &DeviceAuthorization{VerificationURI: loginAuthorizeURL(l.Region, redirect, state, pkceChallenge(verifier)), cancel: lb.stop}
	d.complete = func(ctx context.Context) error {
		defer lb.stop()
		code, err := lb.wait(ctx, state, "aws login")
		if err != nil {
			return err
		}
		out, err := l.token(ctx, key, map[string]string{
			"clientId":     loginClientID,
			"grantType":    "authorization_code",
			"code":         code,
			"redirectUri":  redirect,
			"codeVerifier": verifier,
		})
		if err != nil {
			return err
		}
		session, err := loginSessionFromIDToken(out.IDToken)
		if err != nil {
			return err
		}
		pemKey, err := marshalKey(key)
		if err != nil {
			return err
		}
		return l.store(loginToken{
			AccessKeyID:     out.AccessToken.AccessKeyID,
			SecretAccessKey: out.AccessToken.SecretAccessKey,
			SessionToken:    out.AccessToken.SessionToken,
			Expires:         l.now().Add(time.Duration(out.ExpiresIn) * time.Second),
			RefreshToken:    out.RefreshToken,
			DPoPKey:         pemKey,
			Region:          l.Region,
			LoginSession:    session,
		})
	}
	return d, nil
}

// tokenResponse is the body of a successful /v1/token call.
type tokenResponse struct {
	AccessToken struct {
		AccessKeyID     string `json:"accessKeyId"`
		SecretAccessKey string `json:"secretAccessKey"`
		SessionToken    string `json:"sessionToken"`
	} `json:"accessToken"`
	TokenType    string `json:"tokenType"`
	ExpiresIn    int    `json:"expiresIn"`
	RefreshToken string `json:"refreshToken"`
	IDToken      string `json:"idToken"`
}

// tokenError is the body of a failed /v1/token call.
type tokenError struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// token calls the token endpoint with a DPoP proof for key.
func (l *Login) token(ctx context.Context, key *ecdsa.PrivateKey, body map[string]string) (*tokenResponse, error) {
	endpoint := "https://" + loginHost(l.Region) + "/v1/token"
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	proof, err := dpopProof(key, endpoint, l.now())
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("DPoP", proof)
	resp, err := l.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("aws login: token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("aws login: token: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		var te tokenError
		_ = json.Unmarshal(raw, &te)
		if te.Error == "" {
			te.Error = resp.Header.Get("x-amzn-ErrorType")
		}
		err := fmt.Errorf("aws login: token: %s (%s)", te.Message, te.Error)
		if loginRefused(te.Error) {
			return nil, fmt.Errorf("%w: %v", ErrLoginRequired, err)
		}
		return nil, err
	}
	var out tokenResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("aws login: token: %w", err)
	}
	if out.AccessToken.AccessKeyID == "" || out.RefreshToken == "" {
		return nil, errors.New("aws login: token: incomplete response")
	}
	return &out, nil
}

// loginRefused reports whether a token error means only a new sign-in can
// help. Anything else is a transport or service fault that a retry may fix.
func loginRefused(code string) bool {
	switch code {
	case "TOKEN_EXPIRED", "AUTHCODE_EXPIRED", "USER_CREDENTIALS_CHANGED", "INSUFFICIENT_PERMISSIONS":
		return true
	}
	return false
}

// dpopProof signs a DPoP JWT (RFC 9449) for one POST to uri with an ES256 key.
func dpopProof(key *ecdsa.PrivateKey, uri string, now time.Time) (string, error) {
	enc := func(v any) string {
		b, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(b)
	}
	// The public key is an uncompressed point: 0x04, then X and Y, 32 bytes each.
	point, err := key.PublicKey.Bytes()
	if err != nil {
		return "", err
	}
	header := enc(map[string]any{
		"typ": "dpop+jwt",
		"alg": "ES256",
		"jwk": map[string]string{
			"kty": "EC",
			"crv": "P-256",
			"x":   base64.RawURLEncoding.EncodeToString(point[1:33]),
			"y":   base64.RawURLEncoding.EncodeToString(point[33:65]),
		},
	})
	claims := enc(map[string]any{
		"htm": http.MethodPost,
		"htu": uri,
		"iat": now.Unix(),
		"jti": randomToken(16),
	})
	sum := sha256.Sum256([]byte(header + "." + claims))
	der, err := ecdsa.SignASN1(rand.Reader, key, sum[:])
	if err != nil {
		return "", err
	}
	// JWS wants the raw signature, R and S concatenated, not the ASN.1 form.
	var sig struct{ R, S *big.Int }
	if _, err := asn1.Unmarshal(der, &sig); err != nil {
		return "", err
	}
	raw := append(sig.R.FillBytes(make([]byte, 32)), sig.S.FillBytes(make([]byte, 32))...)
	return header + "." + claims + "." + base64.RawURLEncoding.EncodeToString(raw), nil
}

// marshalKey writes an EC private key as PEM, the form the AWS CLI caches.
func marshalKey(key *ecdsa.PrivateKey) (string, error) {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})), nil
}

func parseKey(s string) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(s))
	if block == nil {
		return nil, errors.New("aws login: stored key is not PEM")
	}
	return x509.ParseECPrivateKey(block.Bytes)
}

// loginSessionFromIDToken reads the sign-in session ARN from the id token's
// subject. The token is not verified: it came over TLS from the token
// endpoint and only names the session.
func loginSessionFromIDToken(idToken string) (string, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return "", errors.New("aws login: id token is not a JWT")
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return "", fmt.Errorf("aws login: id token: %w", err)
	}
	var claims struct {
		Sub string `json:"sub"`
	}
	if err := json.Unmarshal(raw, &claims); err != nil || claims.Sub == "" {
		return "", errors.New("aws login: id token has no subject")
	}
	return claims.Sub, nil
}

// AccountFromLoginSession returns the account ID of a sign-in session ARN,
// arn:aws:signin:{region}:{account}:session/{id}.
func AccountFromLoginSession(arn string) string {
	parts := strings.Split(arn, ":")
	if len(parts) < 6 {
		return ""
	}
	return parts[4]
}

func (l *Login) store(t loginToken) error {
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return l.Secrets.Set(loginTokenKey(l.SessionID), string(data))
}

func (l *Login) stored() (loginToken, error) {
	raw, err := l.Secrets.Get(loginTokenKey(l.SessionID))
	if errors.Is(err, secrets.ErrNotFound) {
		return loginToken{}, ErrLoginRequired
	}
	if err != nil {
		return loginToken{}, err
	}
	var t loginToken
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		return loginToken{}, err
	}
	return t, nil
}

// StoreImported saves a sign-in obtained elsewhere, for example the AWS CLI
// cache, so the session works without a fresh browser login. dpopKey is the
// PEM EC private key the refresh token is bound to.
func (l *Login) StoreImported(creds core.Credentials, refreshToken, dpopKey, loginSession string) error {
	var exp time.Time
	if creds.Expiration != nil {
		exp = creds.Expiration.UTC()
	}
	return l.store(loginToken{
		AccessKeyID: creds.AccessKeyID, SecretAccessKey: creds.SecretAccessKey, SessionToken: creds.SessionToken,
		Expires: exp, RefreshToken: refreshToken, DPoPKey: dpopKey, Region: l.Region, LoginSession: loginSession,
	})
}

// Logout removes the stored sign-in.
func (l *Login) Logout() error { return l.Secrets.Delete(loginTokenKey(l.SessionID)) }

// LoginSession returns the sign-in session ARN of the stored sign-in, or an
// empty string when there is none.
func (l *Login) LoginSession() string {
	t, err := l.stored()
	if err != nil {
		return ""
	}
	return t.LoginSession
}

// valid reports whether the credentials have more than one minute left.
func (l *Login) valid(t loginToken) bool { return l.now().Add(time.Minute).Before(t.Expires) }

// Credentials returns usable credentials. Lapsed ones are renewed with the
// refresh token; a refused refresh means a new sign-in is required.
func (l *Login) Credentials(ctx context.Context) (core.Credentials, error) {
	t, err := l.stored()
	if err != nil {
		return core.Credentials{}, err
	}
	if !l.valid(t) {
		if t, err = l.refresh(ctx, t); err != nil {
			return core.Credentials{}, err
		}
	}
	exp := t.Expires
	return core.Credentials{AccessKeyID: t.AccessKeyID, SecretAccessKey: t.SecretAccessKey, SessionToken: t.SessionToken, Expiration: &exp}, nil
}

// refresh trades the refresh token for new credentials and stores them.
func (l *Login) refresh(ctx context.Context, t loginToken) (loginToken, error) {
	if t.RefreshToken == "" || t.DPoPKey == "" {
		return loginToken{}, ErrLoginRequired
	}
	key, err := parseKey(t.DPoPKey)
	if err != nil {
		return loginToken{}, fmt.Errorf("%w: %v", ErrLoginRequired, err)
	}
	out, err := l.token(ctx, key, map[string]string{
		"clientId":     loginClientID,
		"grantType":    "refresh_token",
		"refreshToken": t.RefreshToken,
	})
	if err != nil {
		return loginToken{}, err
	}
	t.AccessKeyID = out.AccessToken.AccessKeyID
	t.SecretAccessKey = out.AccessToken.SecretAccessKey
	t.SessionToken = out.AccessToken.SessionToken
	t.RefreshToken = out.RefreshToken
	t.Expires = l.now().Add(time.Duration(out.ExpiresIn) * time.Second)
	if err := l.store(t); err != nil {
		return loginToken{}, err
	}
	return t, nil
}

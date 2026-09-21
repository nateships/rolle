// Package aws implements AWS credential providers: IAM Identity Center roles,
// STS AssumeRole chaining, and IAM users.
package aws

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sso"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	oidctypes "github.com/aws/aws-sdk-go-v2/service/ssooidc/types"

	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/netcfg"
	"github.com/nateships/rolle/internal/secrets"
)

const (
	clientName         = "rolle"
	scopeAccountAccess = "sso:account:access"
	redirectPath       = "/oauth/callback"
)

// callbackHTML is the page the browser shows after the loopback redirect.
//
//go:embed callback.html
var callbackHTML string

var callbackPage = template.Must(template.New("callback").Parse(callbackHTML))

// callbackView fills callback.html.
type callbackView struct{ Class, Title, Text string }

// ssoToken is the cached IAM Identity Center access token for one integration.
type ssoToken struct {
	AccessToken  string    `json:"accessToken"`
	RefreshToken string    `json:"refreshToken,omitempty"`
	ClientID     string    `json:"clientId"`
	ClientSecret string    `json:"clientSecret"`
	Expires      time.Time `json:"expires"`
	Region       string    `json:"region"`
}

func ssoTokenKey(integrationID string) string { return "aws-sso-token/" + integrationID }

// ErrSSOLoginRequired is returned when no usable Identity Center token exists.
var ErrSSOLoginRequired = errors.New("aws sso: login required")

// DeviceAuthorization is what the user must do to complete an SSO login.
type DeviceAuthorization struct {
	// VerificationURI is the page to open. For the device flow it embeds the
	// user code; for the browser flow it is the authorization page.
	VerificationURI string
	// UserCode is set for the device flow only.
	UserCode string
	// complete finishes the flow. It is set by StartLogin and StartDeviceLogin.
	complete func(ctx context.Context) error
	// cancel releases the loopback listener of the browser flow. Nil for the device flow.
	cancel func()
}

// Wait blocks until the user approves the login in the browser, then stores the token.
func (d *DeviceAuthorization) Wait(ctx context.Context) error { return d.complete(ctx) }

// Cancel abandons a login that no caller waits on and frees its listener.
func (d *DeviceAuthorization) Cancel() {
	if d.cancel != nil {
		d.cancel()
	}
}

// SSO authenticates against one IAM Identity Center portal.
type SSO struct {
	Integration core.Integration
	Secrets     secrets.Store
	// Now is overridable for tests.
	Now func() time.Time
}

func (s *SSO) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *SSO) cfg(ctx context.Context) (aws.Config, error) {
	return config.LoadDefaultConfig(ctx,
		config.WithRegion(s.Integration.AWSSSO.Region),
		config.WithCredentialsProvider(aws.AnonymousCredentials{}),
		config.WithHTTPClient(netcfg.Client()),
	)
}

// StartLogin begins a browser sign-in with the authorization code flow and
// PKCE. The browser returns to a loopback listener, so the user approves
// without typing a code. It falls back to the device flow when no loopback
// port is available.
func (s *SSO) StartLogin(ctx context.Context) (*DeviceAuthorization, error) {
	state := randomToken(16)
	lb, err := listenLoopback(state)
	if err != nil {
		return s.StartDeviceLogin(ctx)
	}
	cfg, err := s.cfg(ctx)
	if err != nil {
		lb.stop()
		return nil, err
	}
	oidc := ssooidc.NewFromConfig(cfg)
	redirect := lb.redirect
	reg, err := oidc.RegisterClient(ctx, &ssooidc.RegisterClientInput{
		ClientName:   aws.String(clientName),
		ClientType:   aws.String("public"),
		Scopes:       []string{scopeAccountAccess},
		GrantTypes:   []string{"authorization_code", "refresh_token"},
		RedirectUris: []string{redirect},
		IssuerUrl:    aws.String(s.Integration.AWSSSO.StartURL),
	})
	if err != nil {
		lb.stop()
		return nil, fmt.Errorf("register client: %w", err)
	}
	verifier := randomToken(32)
	challenge := pkceChallenge(verifier)

	d := &DeviceAuthorization{VerificationURI: authorizeURL(s.Integration.AWSSSO.Region, aws.ToString(reg.ClientId), redirect, state, challenge), cancel: lb.stop}
	d.complete = func(ctx context.Context) error {
		defer lb.stop()
		code, err := lb.wait(ctx, "aws sso")
		if err != nil {
			return err
		}
		tok, err := oidc.CreateToken(ctx, &ssooidc.CreateTokenInput{
			ClientId:     reg.ClientId,
			ClientSecret: reg.ClientSecret,
			GrantType:    aws.String("authorization_code"),
			Code:         aws.String(code),
			CodeVerifier: aws.String(verifier),
			RedirectUri:  aws.String(redirect),
		})
		if err != nil {
			return fmt.Errorf("create token: %w", err)
		}
		return s.storeToken(ssoToken{
			AccessToken:  aws.ToString(tok.AccessToken),
			RefreshToken: aws.ToString(tok.RefreshToken),
			ClientID:     aws.ToString(reg.ClientId),
			ClientSecret: aws.ToString(reg.ClientSecret),
			Expires:      s.now().Add(time.Duration(tok.ExpiresIn) * time.Second),
			Region:       s.Integration.AWSSSO.Region,
		})
	}
	return d, nil
}

// pkceChallenge is the S256 code challenge for a verifier.
func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// authorizeURL builds the Identity Center authorization page for the PKCE flow.
func authorizeURL(region, clientID, redirect, state, challenge string) string {
	host := "oidc." + region + ".amazonaws.com"
	if strings.HasPrefix(region, "cn-") {
		host += ".cn"
	}
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirect},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"scopes":                {scopeAccountAccess},
	}
	return "https://" + host + "/authorize?" + q.Encode()
}

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// StartDeviceLogin begins the device authorization flow. The caller shows the
// returned verification URI and user code, then calls Wait.
func (s *SSO) StartDeviceLogin(ctx context.Context) (*DeviceAuthorization, error) {
	cfg, err := s.cfg(ctx)
	if err != nil {
		return nil, err
	}
	oidc := ssooidc.NewFromConfig(cfg)
	reg, err := oidc.RegisterClient(ctx, &ssooidc.RegisterClientInput{
		ClientName: aws.String(clientName),
		ClientType: aws.String("public"),
		Scopes:     []string{scopeAccountAccess},
	})
	if err != nil {
		return nil, fmt.Errorf("register client: %w", err)
	}
	auth, err := oidc.StartDeviceAuthorization(ctx, &ssooidc.StartDeviceAuthorizationInput{
		ClientId:     reg.ClientId,
		ClientSecret: reg.ClientSecret,
		StartUrl:     aws.String(s.Integration.AWSSSO.StartURL),
	})
	if err != nil {
		return nil, fmt.Errorf("start device authorization: %w", err)
	}
	interval := time.Duration(auth.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	deadline := s.now().Add(time.Duration(auth.ExpiresIn) * time.Second)

	d := &DeviceAuthorization{
		VerificationURI: aws.ToString(auth.VerificationUriComplete),
		UserCode:        aws.ToString(auth.UserCode),
	}
	d.complete = func(ctx context.Context) error {
		for {
			tok, err := oidc.CreateToken(ctx, &ssooidc.CreateTokenInput{
				ClientId:     reg.ClientId,
				ClientSecret: reg.ClientSecret,
				GrantType:    aws.String("urn:ietf:params:oauth:grant-type:device_code"),
				DeviceCode:   auth.DeviceCode,
			})
			if err == nil {
				return s.storeToken(ssoToken{
					AccessToken:  aws.ToString(tok.AccessToken),
					RefreshToken: aws.ToString(tok.RefreshToken),
					ClientID:     aws.ToString(reg.ClientId),
					ClientSecret: aws.ToString(reg.ClientSecret),
					Expires:      s.now().Add(time.Duration(tok.ExpiresIn) * time.Second),
					Region:       s.Integration.AWSSSO.Region,
				})
			}
			var pending *oidctypes.AuthorizationPendingException
			var slow *oidctypes.SlowDownException
			switch {
			case errors.As(err, &pending):
			case errors.As(err, &slow):
				interval += 5 * time.Second
			default:
				return fmt.Errorf("create token: %w", err)
			}
			if s.now().After(deadline) {
				return errors.New("aws sso: login timed out")
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(interval):
			}
		}
	}
	return d, nil
}

func (s *SSO) storeToken(t ssoToken) error {
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return s.Secrets.Set(ssoTokenKey(s.Integration.ID), string(data))
}

// StoreImportedToken saves a token obtained elsewhere, for example the AWS CLI
// cache, so the integration works without a fresh device login.
func (s *SSO) StoreImportedToken(accessToken, refreshToken, clientID, clientSecret, region string, expires time.Time) error {
	if region == "" {
		region = s.Integration.AWSSSO.Region
	}
	return s.storeToken(ssoToken{
		AccessToken: accessToken, RefreshToken: refreshToken, ClientID: clientID, ClientSecret: clientSecret,
		Expires: expires.UTC(), Region: region,
	})
}

// Logout removes the cached token.
func (s *SSO) Logout() error { return s.Secrets.Delete(ssoTokenKey(s.Integration.ID)) }

// LoginState reads the stored token once and reports when the access token
// stops working, and whether a refresh token renews it without the browser.
// expires is nil when only a new login can produce credentials. A token past
// its expiry still counts while a refresh token can renew it; the time is
// then in the past.
func (s *SSO) LoginState() (expires *time.Time, renews bool) {
	t, err := s.storedToken()
	if err != nil || (!s.valid(t) && !refreshable(t)) {
		return nil, false
	}
	return &t.Expires, refreshable(t)
}

// TokenExpiry returns when the access token stops working, or nil when only a
// new login can produce credentials. See LoginState.
func (s *SSO) TokenExpiry() *time.Time {
	expires, _ := s.LoginState()
	return expires
}

// Renews reports whether the stored token carries what a silent refresh needs.
func (s *SSO) Renews() bool {
	_, renews := s.LoginState()
	return renews
}

// Probe reports whether the stored portal token still works and returns its
// expiry. A lapsed access token is refreshed, so the call proves the login
// rather than the record of it. ErrSSOLoginRequired means only a new login
// can help; any other error is a transport or service fault.
func (s *SSO) Probe(ctx context.Context) (*time.Time, error) {
	t, err := s.token(ctx)
	if err != nil {
		return nil, err
	}
	return &t.Expires, nil
}

// valid reports whether the access token has more than one minute left.
func (s *SSO) valid(t ssoToken) bool { return s.now().Add(time.Minute).Before(t.Expires) }

// refreshable reports whether the portal issued what a refresh_token grant needs.
func refreshable(t ssoToken) bool {
	return t.RefreshToken != "" && t.ClientID != "" && t.ClientSecret != ""
}

func (s *SSO) storedToken() (ssoToken, error) {
	raw, err := s.Secrets.Get(ssoTokenKey(s.Integration.ID))
	if errors.Is(err, secrets.ErrNotFound) {
		return ssoToken{}, ErrSSOLoginRequired
	}
	if err != nil {
		return ssoToken{}, err
	}
	var t ssoToken
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		return ssoToken{}, err
	}
	return t, nil
}

// token returns a usable access token. An expired token is renewed with the
// refresh token when the portal issued one; otherwise a login is required.
func (s *SSO) token(ctx context.Context) (ssoToken, error) {
	t, err := s.storedToken()
	if err != nil {
		return ssoToken{}, err
	}
	if s.valid(t) {
		return t, nil
	}
	if !refreshable(t) {
		return ssoToken{}, ErrSSOLoginRequired
	}
	return s.refresh(ctx, t)
}

// refreshRejected reports whether the OIDC service refused the refresh token.
// Only a refusal needs a new login. A transport error or a service fault does
// not, and the next renewal retries it.
func refreshRejected(err error) bool {
	var (
		invalidGrant  *oidctypes.InvalidGrantException
		expired       *oidctypes.ExpiredTokenException
		invalidClient *oidctypes.InvalidClientException
		unauthorized  *oidctypes.UnauthorizedClientException
	)
	return errors.As(err, &invalidGrant) || errors.As(err, &expired) || errors.As(err, &invalidClient) || errors.As(err, &unauthorized)
}

// refresh trades the refresh token for a new access token and stores it.
func (s *SSO) refresh(ctx context.Context, t ssoToken) (ssoToken, error) {
	cfg, err := s.cfg(ctx)
	if err != nil {
		return ssoToken{}, err
	}
	out, err := ssooidc.NewFromConfig(cfg).CreateToken(ctx, &ssooidc.CreateTokenInput{
		ClientId:     aws.String(t.ClientID),
		ClientSecret: aws.String(t.ClientSecret),
		GrantType:    aws.String("refresh_token"),
		RefreshToken: aws.String(t.RefreshToken),
	})
	if err != nil {
		if refreshRejected(err) {
			return ssoToken{}, fmt.Errorf("%w: token refresh failed: %v", ErrSSOLoginRequired, err)
		}
		return ssoToken{}, fmt.Errorf("aws sso: token refresh failed: %w", err)
	}
	t.AccessToken = aws.ToString(out.AccessToken)
	if rt := aws.ToString(out.RefreshToken); rt != "" {
		t.RefreshToken = rt
	}
	t.Expires = s.now().Add(time.Duration(out.ExpiresIn) * time.Second)
	if err := s.storeToken(t); err != nil {
		return ssoToken{}, err
	}
	return t, nil
}

// Account is an AWS account visible through the portal.
type Account struct {
	ID   string
	Name string
}

// Role is a permission set assignment in one account.
type Role struct {
	AccountID string
	Name      string
}

// ListAccounts returns every account the signed-in user can access.
func (s *SSO) ListAccounts(ctx context.Context) ([]Account, error) {
	tok, err := s.token(ctx)
	if err != nil {
		return nil, err
	}
	cfg, err := s.cfg(ctx)
	if err != nil {
		return nil, err
	}
	client := sso.NewFromConfig(cfg)
	var out []Account
	p := sso.NewListAccountsPaginator(client, &sso.ListAccountsInput{AccessToken: aws.String(tok.AccessToken)})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list accounts: %w", err)
		}
		for _, a := range page.AccountList {
			out = append(out, Account{ID: aws.ToString(a.AccountId), Name: aws.ToString(a.AccountName)})
		}
	}
	return out, nil
}

// ListRoles returns the roles available in one account.
func (s *SSO) ListRoles(ctx context.Context, accountID string) ([]Role, error) {
	tok, err := s.token(ctx)
	if err != nil {
		return nil, err
	}
	cfg, err := s.cfg(ctx)
	if err != nil {
		return nil, err
	}
	client := sso.NewFromConfig(cfg)
	var out []Role
	p := sso.NewListAccountRolesPaginator(client, &sso.ListAccountRolesInput{
		AccessToken: aws.String(tok.AccessToken),
		AccountId:   aws.String(accountID),
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list roles: %w", err)
		}
		for _, r := range page.RoleList {
			out = append(out, Role{AccountID: accountID, Name: aws.ToString(r.RoleName)})
		}
	}
	return out, nil
}

// RoleCredentials fetches short-lived credentials for one SSO role.
func (s *SSO) RoleCredentials(ctx context.Context, accountID, roleName string) (core.Credentials, error) {
	tok, err := s.token(ctx)
	if err != nil {
		return core.Credentials{}, err
	}
	cfg, err := s.cfg(ctx)
	if err != nil {
		return core.Credentials{}, err
	}
	out, err := sso.NewFromConfig(cfg).GetRoleCredentials(ctx, &sso.GetRoleCredentialsInput{
		AccessToken: aws.String(tok.AccessToken),
		AccountId:   aws.String(accountID),
		RoleName:    aws.String(roleName),
	})
	if err != nil {
		return core.Credentials{}, fmt.Errorf("get role credentials: %w", err)
	}
	rc := out.RoleCredentials
	if rc == nil {
		return core.Credentials{}, errors.New("get role credentials: empty response")
	}
	exp := time.UnixMilli(rc.Expiration).UTC()
	return core.Credentials{
		AccessKeyID:     aws.ToString(rc.AccessKeyId),
		SecretAccessKey: aws.ToString(rc.SecretAccessKey),
		SessionToken:    aws.ToString(rc.SessionToken),
		Expiration:      &exp,
	}, nil
}

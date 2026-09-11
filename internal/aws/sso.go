// Package aws implements AWS credential providers: IAM Identity Center roles,
// STS AssumeRole chaining, and IAM users.
package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sso"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	oidctypes "github.com/aws/aws-sdk-go-v2/service/ssooidc/types"

	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/secrets"
)

const clientName = "rolle"

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
	// VerificationURI is the page to open. It already embeds the user code.
	VerificationURI string
	UserCode        string
	// complete finishes the flow. It is set by StartSSOLogin.
	complete func(ctx context.Context) error
}

// Wait blocks until the user approves the login in the browser, then stores the token.
func (d *DeviceAuthorization) Wait(ctx context.Context) error { return d.complete(ctx) }

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
	)
}

// StartLogin begins the device authorization flow. The caller shows the returned
// verification URI to the user and then calls Wait.
func (s *SSO) StartLogin(ctx context.Context) (*DeviceAuthorization, error) {
	cfg, err := s.cfg(ctx)
	if err != nil {
		return nil, err
	}
	oidc := ssooidc.NewFromConfig(cfg)
	reg, err := oidc.RegisterClient(ctx, &ssooidc.RegisterClientInput{
		ClientName: aws.String(clientName),
		ClientType: aws.String("public"),
		Scopes:     []string{"sso:account:access"},
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
// cache, so this integration can be used without a fresh device login.
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

// TokenExpiry returns when the cached token expires, or nil when logged out.
func (s *SSO) TokenExpiry() *time.Time {
	t, err := s.token()
	if err != nil {
		return nil
	}
	return &t.Expires
}

func (s *SSO) token() (ssoToken, error) {
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
	if !s.now().Add(time.Minute).Before(t.Expires) {
		return ssoToken{}, ErrSSOLoginRequired
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
	tok, err := s.token()
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
	tok, err := s.token()
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
	tok, err := s.token()
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
	exp := time.UnixMilli(rc.Expiration).UTC()
	return core.Credentials{
		AccessKeyID:     aws.ToString(rc.AccessKeyId),
		SecretAccessKey: aws.ToString(rc.SecretAccessKey),
		SessionToken:    aws.ToString(rc.SessionToken),
		Expiration:      &exp,
	}, nil
}

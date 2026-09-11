package azure

import (
	"context"
	"errors"
	"fmt"

	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/cache"
	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/public"

	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/secrets"
)

// ClientID is the Azure CLI public client. Reusing it means no app registration
// is needed and the localhost redirect is already allowed.
const ClientID = "04b07795-8ddb-461a-bbee-02f9e1bf7b46"

// ErrLoginRequired is returned when no account is signed in for a tenant.
var ErrLoginRequired = errors.New("azure: login required")

// Auth signs in to one Entra ID tenant and mints tokens from the cached account.
type Auth struct {
	Integration core.Integration
	Secrets     secrets.Store
}

func cacheKey(integrationID string) string { return "azure-msal-cache/" + integrationID }

// secretCache persists the MSAL token cache in the secret store.
type secretCache struct {
	store secrets.Store
	key   string
}

func (c secretCache) Replace(_ context.Context, u cache.Unmarshaler, _ cache.ReplaceHints) error {
	raw, err := c.store.Get(c.key)
	if errors.Is(err, secrets.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return u.Unmarshal([]byte(raw))
}

func (c secretCache) Export(_ context.Context, m cache.Marshaler, _ cache.ExportHints) error {
	data, err := m.Marshal()
	if err != nil {
		return err
	}
	return c.store.Set(c.key, string(data))
}

func (a *Auth) tenant() string {
	if a.Integration.Azure != nil && a.Integration.Azure.TenantID != "" {
		return a.Integration.Azure.TenantID
	}
	return "organizations"
}

func (a *Auth) client() (public.Client, error) {
	return public.New(ClientID,
		public.WithAuthority("https://login.microsoftonline.com/"+a.tenant()),
		public.WithCache(secretCache{store: a.Secrets, key: cacheKey(a.Integration.ID)}),
	)
}

// DeviceCode is shown to the user when the browser flow is unavailable.
type DeviceCode struct {
	Message         string
	UserCode        string
	VerificationURL string
	result          func(ctx context.Context) (public.AuthResult, error)
}

// Login signs in interactively through the system browser. It returns the
// signed-in account name.
func (a *Auth) Login(ctx context.Context) (string, error) {
	client, err := a.client()
	if err != nil {
		return "", err
	}
	res, err := client.AcquireTokenInteractive(ctx, []string{ARMScope}, public.WithRedirectURI("http://localhost"))
	if err != nil {
		return "", fmt.Errorf("azure login: %w", err)
	}
	return res.Account.PreferredUsername, nil
}

// StartDeviceLogin begins the device-code flow for headless environments.
func (a *Auth) StartDeviceLogin(ctx context.Context) (*DeviceCode, error) {
	client, err := a.client()
	if err != nil {
		return nil, err
	}
	dc, err := client.AcquireTokenByDeviceCode(ctx, []string{ARMScope})
	if err != nil {
		return nil, fmt.Errorf("azure device login: %w", err)
	}
	return &DeviceCode{
		Message:         dc.Result.Message,
		UserCode:        dc.Result.UserCode,
		VerificationURL: dc.Result.VerificationURL,
		result:          dc.AuthenticationResult,
	}, nil
}

// Wait completes a device-code login and returns the account name.
func (d *DeviceCode) Wait(ctx context.Context) (string, error) {
	res, err := d.result(ctx)
	if err != nil {
		return "", err
	}
	return res.Account.PreferredUsername, nil
}

// Account returns the cached account name, or "" when logged out.
func (a *Auth) Account(ctx context.Context) string {
	client, err := a.client()
	if err != nil {
		return ""
	}
	accts, err := client.Accounts(ctx)
	if err != nil || len(accts) == 0 {
		return ""
	}
	return accts[0].PreferredUsername
}

// Token returns an ARM access token, refreshing silently.
func (a *Auth) Token(ctx context.Context) (core.Credentials, error) {
	client, err := a.client()
	if err != nil {
		return core.Credentials{}, err
	}
	accts, err := client.Accounts(ctx)
	if err != nil {
		return core.Credentials{}, err
	}
	if len(accts) == 0 {
		return core.Credentials{}, ErrLoginRequired
	}
	res, err := client.AcquireTokenSilent(ctx, []string{ARMScope}, public.WithSilentAccount(accts[0]))
	if err != nil {
		return core.Credentials{}, fmt.Errorf("%w: %v", ErrLoginRequired, err)
	}
	exp := res.ExpiresOn.UTC()
	return core.Credentials{Token: res.AccessToken, Expiration: &exp}, nil
}

// Logout forgets every cached account.
func (a *Auth) Logout(ctx context.Context) error {
	client, err := a.client()
	if err != nil {
		return err
	}
	accts, err := client.Accounts(ctx)
	if err == nil {
		for _, acct := range accts {
			_ = client.RemoveAccount(ctx, acct)
		}
	}
	return a.Secrets.Delete(cacheKey(a.Integration.ID))
}

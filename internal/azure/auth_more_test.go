package azure

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/public"

	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/secrets"
)

// cachedAccounts is an MSAL cache with two signed-in accounts, in the JSON
// layout MSAL shares across languages.
const cachedAccounts = `{"Account": {
 "a-1.t-1-login.microsoftonline.com-t-1": {"home_account_id": "a-1.t-1", "environment": "login.microsoftonline.com", "realm": "t-1", "local_account_id": "a-1", "authority_type": "MSSTS", "username": "first@contoso.com"},
 "a-2.t-1-login.microsoftonline.com-t-1": {"home_account_id": "a-2.t-1", "environment": "login.microsoftonline.com", "realm": "t-1", "local_account_id": "a-2", "authority_type": "MSSTS", "username": "me@contoso.com"}
}}`

// seededAuth returns an Auth whose secret store holds cachedAccounts.
func seededAuth(t *testing.T) (*Auth, *secrets.Memory) {
	t.Helper()
	mem := &secrets.Memory{}
	if err := mem.Set(cacheKey("i1"), cachedAccounts); err != nil {
		t.Fatal(err)
	}
	a := &Auth{Integration: core.Integration{ID: "i1", Azure: &core.AzureIntegration{TenantID: "t-1", Account: "me@contoso.com"}}, Secrets: mem}
	return a, mem
}

// cancelled returns a context that is already done, so no request leaves
// the process.
func cancelled() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestTokenForWithCachedAccountAndNoNetworkIsTransient(t *testing.T) {
	a, _ := seededAuth(t)
	// The identity service never answers on a cancelled context. That is a
	// transient failure: the user keeps the sign-in and the next renewal
	// tries again.
	_, err := a.TokenFor(cancelled(), AKSScope)
	if err == nil || errors.Is(err, ErrLoginRequired) {
		t.Fatalf("TokenFor offline = %v, want a transient error", err)
	}
	if !errors.Is(err, context.Canceled) || !strings.HasPrefix(err.Error(), "azure token: ") {
		t.Fatalf("TokenFor offline = %v", err)
	}
	// An integration without a recorded account takes the first one.
	a.Integration.Azure = nil
	if _, err := a.Token(cancelled()); err == nil || errors.Is(err, ErrLoginRequired) {
		t.Fatalf("Token without a recorded account = %v", err)
	}
}

func TestLogoutForgetsCachedAccounts(t *testing.T) {
	a, mem := seededAuth(t)
	if err := a.Logout(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Get(cacheKey("i1")); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatal("MSAL cache entry kept after logout")
	}
	if _, err := a.Token(context.Background()); !errors.Is(err, ErrLoginRequired) {
		t.Fatalf("Token after logout = %v, want ErrLoginRequired", err)
	}
}

func TestStartDeviceLoginNeedsTheIdentityService(t *testing.T) {
	a := &Auth{Integration: core.Integration{ID: "i1", Azure: &core.AzureIntegration{TenantID: "t-1"}}, Secrets: &secrets.Memory{}}
	_, err := a.StartDeviceLogin(cancelled())
	if err == nil || !strings.HasPrefix(err.Error(), "azure device login: ") || !errors.Is(err, context.Canceled) {
		t.Fatalf("StartDeviceLogin offline = %v", err)
	}
}

func TestDeviceCodeWaitReturnsTheAccountName(t *testing.T) {
	dc := &DeviceCode{result: func(context.Context) (public.AuthResult, error) {
		res := public.AuthResult{}
		res.Account.PreferredUsername = "me@contoso.com"
		return res, nil
	}}
	if got, err := dc.Wait(context.Background()); err != nil || got != "me@contoso.com" {
		t.Fatalf("Wait = %q, %v", got, err)
	}
	boom := errors.New("authorization_pending")
	dc = &DeviceCode{result: func(context.Context) (public.AuthResult, error) { return public.AuthResult{}, boom }}
	if _, err := dc.Wait(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("Wait error = %v", err)
	}
}

func TestMalformedTenantFailsBeforeAnyRequest(t *testing.T) {
	// A control character makes the authority URL unparsable, so every entry
	// point fails at client construction and never reaches the network.
	a := &Auth{Integration: core.Integration{ID: "i1", Azure: &core.AzureIntegration{TenantID: "bad\x7ftenant"}}, Secrets: &secrets.Memory{}}
	ctx := context.Background()
	if _, err := a.Login(ctx); err == nil {
		t.Fatal("Login accepted a malformed tenant")
	}
	if _, err := a.StartDeviceLogin(ctx); err == nil {
		t.Fatal("StartDeviceLogin accepted a malformed tenant")
	}
	if _, err := a.Token(ctx); err == nil || errors.Is(err, ErrLoginRequired) {
		t.Fatalf("Token with a malformed tenant = %v", err)
	}
	if err := a.Logout(ctx); err == nil {
		t.Fatal("Logout accepted a malformed tenant")
	}
}

func TestARMListReportsRequestFailures(t *testing.T) {
	var out []Subscription
	if err := ARMList(context.Background(), http.DefaultClient, "tok", "::bad url", &out); err == nil {
		t.Fatal("malformed URL accepted")
	}
	if err := ARMList(context.Background(), http.DefaultClient, "tok", "http://127.0.0.1:1/subscriptions", &out); err == nil {
		t.Fatal("unreachable host accepted")
	}
	// A page that fails after the first one is an error, not a partial list.
	client, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("skiptoken") == "" {
			fmt.Fprint(w, `{"value":[{"subscriptionId":"s1"}],"nextLink":"https://management.azure.com/subscriptions?skiptoken=n2"}`)
			return
		}
		http.Error(w, `{"error":"throttled"}`, http.StatusTooManyRequests)
	})
	_, err := ListSubscriptions(context.Background(), client, "tok")
	if err == nil || !strings.Contains(err.Error(), "429") || !strings.Contains(err.Error(), "throttled") {
		t.Fatalf("second page error = %v", err)
	}
}

package aws

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nateships/rolle/internal/secrets"
)

// fakePortal serves the Identity Center portal API and its OIDC endpoints on
// one server. Requests are recorded by path.
type fakePortal struct {
	paths     []string
	token     string // the bearer token the portal accepts
	tokenFail string // X-Amzn-ErrorType for /token, empty for success
	devFail   bool   // refuse /device_authorization
	regFail   bool   // refuse /client/register
}

func (f *fakePortal) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.paths = append(f.paths, r.URL.Path)
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/client/register":
		if f.regFail {
			refuse(w, "InvalidClientMetadataException")
			return
		}
		fmt.Fprint(w, `{"clientId":"cid","clientSecret":"cs","clientIdIssuedAt":1,"clientSecretExpiresAt":9999999999}`)
	case "/device_authorization":
		if f.devFail {
			refuse(w, "InvalidRequestException")
			return
		}
		fmt.Fprint(w, `{"deviceCode":"dev-code","userCode":"ABCD-EFGH","verificationUri":"https://device.sso.example/","verificationUriComplete":"https://device.sso.example/?user_code=ABCD-EFGH","expiresIn":600,"interval":1}`)
	case "/token":
		if f.tokenFail != "" {
			refuse(w, f.tokenFail)
			return
		}
		fmt.Fprintf(w, `{"accessToken":%q,"refreshToken":"rt-new","expiresIn":3600,"tokenType":"Bearer"}`, f.token)
	case "/assignment/accounts":
		if !f.authorized(w, r) {
			return
		}
		if r.URL.Query().Get("next_token") == "" {
			fmt.Fprint(w, `{"accountList":[{"accountId":"111111111111","accountName":"Acme"}],"nextToken":"p2"}`)
		} else {
			fmt.Fprint(w, `{"accountList":[{"accountId":"222222222222","accountName":"Acme Dev"}]}`)
		}
	case "/assignment/roles":
		if !f.authorized(w, r) {
			return
		}
		acct := r.URL.Query().Get("account_id")
		fmt.Fprintf(w, `{"roleList":[{"accountId":%q,"roleName":"Admin"},{"accountId":%q,"roleName":"ReadOnly"}]}`, acct, acct)
	case "/federation/credentials":
		if !f.authorized(w, r) {
			return
		}
		q := r.URL.Query()
		fmt.Fprintf(w, `{"roleCredentials":{"accessKeyId":"ASIA-%s-%s","secretAccessKey":"sec","sessionToken":"tok","expiration":%d}}`, q.Get("account_id"), q.Get("role_name"), time.Date(2026, 1, 1, 13, 0, 0, 0, time.UTC).UnixMilli())
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *fakePortal) authorized(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("x-amz-sso_bearer_token") == f.token {
		return true
	}
	w.Header().Set("X-Amzn-ErrorType", "UnauthorizedException")
	w.WriteHeader(http.StatusUnauthorized)
	fmt.Fprint(w, `{"message":"Session token not found or invalid"}`)
	return false
}

func refuse(w http.ResponseWriter, code string) {
	w.Header().Set("X-Amzn-ErrorType", code)
	w.WriteHeader(http.StatusBadRequest)
	fmt.Fprintf(w, `{"error":%q,"error_description":"refused by test"}`, strings.ToLower(code))
}

func startFakePortal(t *testing.T) *fakePortal {
	t.Helper()
	isolateAWS(t)
	f := &fakePortal{token: "portal-token"}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	t.Setenv("AWS_ENDPOINT_URL_SSO", srv.URL)
	t.Setenv("AWS_ENDPOINT_URL_SSO_OIDC", srv.URL)
	return f
}

// loggedIn returns an SSO with a valid stored access token.
func loggedIn(t *testing.T, now *time.Time) (*SSO, *secrets.Memory) {
	t.Helper()
	mem := &secrets.Memory{}
	s := newSSO(mem, now)
	if err := s.StoreImportedToken("portal-token", "", "", "", "", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	return s, mem
}

func TestListAccountsFollowsPagination(t *testing.T) {
	portal := startFakePortal(t)
	now := ssoNow
	s, _ := loggedIn(t, &now)
	got, err := s.ListAccounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != (Account{ID: "111111111111", Name: "Acme"}) || got[1] != (Account{ID: "222222222222", Name: "Acme Dev"}) {
		t.Fatalf("accounts = %+v", got)
	}
	if n := strings.Count(strings.Join(portal.paths, " "), "/assignment/accounts"); n != 2 {
		t.Fatalf("%d account pages fetched, want 2", n)
	}
}

func TestListRolesAndRoleCredentials(t *testing.T) {
	startFakePortal(t)
	now := ssoNow
	s, _ := loggedIn(t, &now)
	roles, err := s.ListRoles(context.Background(), "111111111111")
	if err != nil {
		t.Fatal(err)
	}
	if len(roles) != 2 || roles[0] != (Role{AccountID: "111111111111", Name: "Admin"}) || roles[1].Name != "ReadOnly" {
		t.Fatalf("roles = %+v", roles)
	}
	creds, err := s.RoleCredentials(context.Background(), "111111111111", "Admin")
	if err != nil {
		t.Fatal(err)
	}
	wantExp := time.Date(2026, 1, 1, 13, 0, 0, 0, time.UTC)
	if creds.AccessKeyID != "ASIA-111111111111-Admin" || creds.SecretAccessKey != "sec" || creds.SessionToken != "tok" || creds.Expiration == nil || !creds.Expiration.Equal(wantExp) {
		t.Fatalf("creds = %+v", creds)
	}
}

func TestPortalRefusalIsReportedPerCall(t *testing.T) {
	portal := startFakePortal(t)
	portal.token = "someone-else"
	now := ssoNow
	s, _ := loggedIn(t, &now)
	ctx := context.Background()
	if _, err := s.ListAccounts(ctx); err == nil || !strings.HasPrefix(err.Error(), "list accounts: ") {
		t.Fatalf("ListAccounts = %v", err)
	}
	if _, err := s.ListRoles(ctx, "1"); err == nil || !strings.HasPrefix(err.Error(), "list roles: ") {
		t.Fatalf("ListRoles = %v", err)
	}
	if _, err := s.RoleCredentials(ctx, "1", "Admin"); err == nil || !strings.HasPrefix(err.Error(), "get role credentials: ") {
		t.Fatalf("RoleCredentials = %v", err)
	}
}

func TestDeviceLoginStoresTokenOnApproval(t *testing.T) {
	portal := startFakePortal(t)
	now := ssoNow
	mem := &secrets.Memory{}
	s := newSSO(mem, &now)
	auth, err := s.StartDeviceLogin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if auth.UserCode != "ABCD-EFGH" || auth.VerificationURI != "https://device.sso.example/?user_code=ABCD-EFGH" {
		t.Fatalf("auth = %+v", auth)
	}
	auth.Cancel() // the device flow has no listener to free
	if _, err := mem.Get("aws-sso-token/i1"); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatal("token stored before the user approved")
	}
	if err := auth.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	tok, err := s.storedToken()
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "portal-token" || tok.RefreshToken != "rt-new" || tok.ClientID != "cid" || tok.ClientSecret != "cs" || tok.Region != "us-east-1" || !tok.Expires.Equal(now.Add(time.Hour)) {
		t.Fatalf("stored token = %+v", tok)
	}
	if got := strings.Join(portal.paths, " "); got != "/client/register /device_authorization /token" {
		t.Fatalf("calls = %q", got)
	}
	// The stored token works against the portal straight away.
	if _, err := s.ListAccounts(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDeviceLoginErrors(t *testing.T) {
	portal := startFakePortal(t)
	now := ssoNow
	s := newSSO(&secrets.Memory{}, &now)

	portal.regFail = true
	if _, err := s.StartDeviceLogin(context.Background()); err == nil || !strings.HasPrefix(err.Error(), "register client: ") {
		t.Fatalf("register failure = %v", err)
	}
	portal.regFail, portal.devFail = false, true
	if _, err := s.StartDeviceLogin(context.Background()); err == nil || !strings.HasPrefix(err.Error(), "start device authorization: ") {
		t.Fatalf("device authorization failure = %v", err)
	}
	portal.devFail = false
	auth, err := s.StartDeviceLogin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	portal.tokenFail = "AccessDeniedException"
	if err := auth.Wait(context.Background()); err == nil || !strings.HasPrefix(err.Error(), "create token: ") {
		t.Fatalf("denied approval = %v", err)
	}
	if s.TokenExpiry() != nil {
		t.Fatal("a refused login must not store a token")
	}
}

func TestRefreshClassifiesRefusalAndFault(t *testing.T) {
	portal := startFakePortal(t)
	now := ssoNow
	mem := &secrets.Memory{}
	s := newSSO(mem, &now)
	expired := func() {
		if err := s.StoreImportedToken("old", "rt", "cid", "cs", "", now.Add(-time.Minute)); err != nil {
			t.Fatal(err)
		}
	}

	expired()
	portal.tokenFail = "InvalidGrantException"
	_, err := s.token(context.Background())
	if !errors.Is(err, ErrSSOLoginRequired) {
		t.Fatalf("invalid_grant = %v, want login required", err)
	}
	if s.TokenExpiry() == nil {
		t.Fatal("the stored token still has a refresh token, so the login counts until the portal refuses it")
	}

	portal.tokenFail = "InternalServerException"
	_, err = s.token(context.Background())
	if err == nil || errors.Is(err, ErrSSOLoginRequired) || !strings.Contains(err.Error(), "token refresh failed") {
		t.Fatalf("service fault = %v, want a retryable error", err)
	}

	portal.tokenFail = ""
	tok, err := s.token(context.Background())
	if err != nil || tok.AccessToken != "portal-token" || tok.RefreshToken != "rt-new" {
		t.Fatalf("refreshed = %+v, %v", tok, err)
	}
	// RoleCredentials refreshes on its own before calling the portal.
	expired()
	if _, err := s.RoleCredentials(context.Background(), "1", "Admin"); err != nil {
		t.Fatal(err)
	}
}

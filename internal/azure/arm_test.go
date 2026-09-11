package azure

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/cache"

	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/secrets"
)

// rewriteTransport sends every request to a test server, whatever host the
// code under test asked for.
type rewriteTransport struct {
	target *url.URL
	calls  int
}

func (r *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.calls++
	req = req.Clone(req.Context())
	req.URL.Scheme = r.target.Scheme
	req.URL.Host = r.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

func testClient(t *testing.T, handler http.HandlerFunc) (*http.Client, *rewriteTransport) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	rt := &rewriteTransport{target: target}
	return &http.Client{Transport: rt}, rt
}

func TestListSubscriptionsPaginates(t *testing.T) {
	client, rt := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" || r.URL.Path != "/subscriptions" {
			t.Errorf("unexpected request %s %v", r.URL, r.Header)
		}
		switch r.URL.Query().Get("skiptoken") {
		case "":
			if r.URL.Query().Get("api-version") != "2022-12-01" {
				t.Errorf("api-version missing: %s", r.URL)
			}
			fmt.Fprint(w, `{"value":[{"subscriptionId":"s1","displayName":"One","state":"Enabled","tenantId":"t1"}],"nextLink":"https://management.azure.com/subscriptions?skiptoken=n2"}`)
		case "n2":
			fmt.Fprint(w, `{"value":[{"subscriptionId":"s2","displayName":"Two","state":"Disabled","tenantId":"t2"}]}`)
		default:
			t.Errorf("unexpected skiptoken %q", r.URL.Query().Get("skiptoken"))
		}
	})
	got, err := ListSubscriptions(context.Background(), client, "tok")
	if err != nil {
		t.Fatal(err)
	}
	want := []Subscription{
		{ID: "s1", Name: "One", State: "Enabled", TenantID: "t1"},
		{ID: "s2", Name: "Two", State: "Disabled", TenantID: "t2"},
	}
	if !reflect.DeepEqual(got, want) || rt.calls != 2 {
		t.Fatalf("subscriptions = %+v (%d calls)", got, rt.calls)
	}
}

func TestListSubscriptionsErrors(t *testing.T) {
	client, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"InvalidAuthenticationToken"}`, http.StatusUnauthorized)
	})
	_, err := ListSubscriptions(context.Background(), client, "tok")
	if err == nil || !strings.Contains(err.Error(), "azure resource manager") || !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "InvalidAuthenticationToken") {
		t.Fatalf("err = %v", err)
	}
	client, _ = testClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "not json")
	})
	if _, err := ListSubscriptions(context.Background(), client, "tok"); err == nil {
		t.Fatal("expected a JSON error")
	}
	client, _ = testClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{}`)
	})
	got, err := ListSubscriptions(context.Background(), client, "tok")
	if err != nil || len(got) != 0 {
		t.Fatalf("empty page = %+v, %v", got, err)
	}
}

func TestTenantDefaultsToOrganizations(t *testing.T) {
	cases := []struct {
		name string
		azr  *core.AzureIntegration
		want string
	}{
		{"nil integration settings", nil, "organizations"},
		{"empty tenant", &core.AzureIntegration{}, "organizations"},
		{"explicit tenant", &core.AzureIntegration{TenantID: "t-123"}, "t-123"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := &Auth{Integration: core.Integration{ID: "i1", Azure: tc.azr}}
			if got := a.tenant(); got != tc.want {
				t.Fatalf("tenant = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCacheKey(t *testing.T) {
	if got := cacheKey("i1"); got != "azure-msal-cache/i1" {
		t.Fatalf("cacheKey = %q", got)
	}
}

// failStore returns err from every call.
type failStore struct{ err error }

func (f failStore) Get(string) (string, error) { return "", f.err }
func (f failStore) Set(string, string) error   { return f.err }
func (f failStore) Delete(string) error        { return f.err }

type fakeUnmarshaler struct {
	called bool
	data   string
}

func (u *fakeUnmarshaler) Unmarshal(b []byte) error {
	u.called = true
	u.data = string(b)
	return nil
}

type fakeMarshaler struct {
	data string
	err  error
}

func (m fakeMarshaler) Marshal() ([]byte, error) { return []byte(m.data), m.err }

func TestSecretCacheReplaceAndExport(t *testing.T) {
	ctx := context.Background()
	mem := &secrets.Memory{}
	c := secretCache{store: mem, key: "k"}

	var u fakeUnmarshaler
	if err := c.Replace(ctx, &u, cache.ReplaceHints{}); err != nil || u.called {
		t.Fatalf("Replace with empty store: err = %v, called = %v", err, u.called)
	}
	if err := c.Export(ctx, fakeMarshaler{data: "blob"}, cache.ExportHints{}); err != nil {
		t.Fatal(err)
	}
	if got, _ := mem.Get("k"); got != "blob" {
		t.Fatalf("stored = %q", got)
	}
	if err := c.Replace(ctx, &u, cache.ReplaceHints{}); err != nil || !u.called || u.data != "blob" {
		t.Fatalf("Replace after Export: err = %v, data = %q", err, u.data)
	}

	boom := errors.New("boom")
	if err := c.Export(ctx, fakeMarshaler{err: boom}, cache.ExportHints{}); !errors.Is(err, boom) {
		t.Fatalf("marshal error = %v", err)
	}
	failing := secretCache{store: failStore{boom}, key: "k"}
	if err := failing.Replace(ctx, &u, cache.ReplaceHints{}); !errors.Is(err, boom) {
		t.Fatalf("store error on Replace = %v", err)
	}
	if err := failing.Export(ctx, fakeMarshaler{data: "x"}, cache.ExportHints{}); !errors.Is(err, boom) {
		t.Fatalf("store error on Export = %v", err)
	}
}

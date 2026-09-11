package netcfg

import (
	"crypto/tls"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nateships/rolle/internal/core"
)

func restore(t *testing.T) {
	t.Helper()
	prev := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = prev })
}

func TestApplyTrustsExtraBundle(t *testing.T) {
	restore(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	defer srv.Close()
	// Without the bundle the self-signed server is rejected.
	if err := Apply(core.Settings{}); err != nil {
		t.Fatal(err)
	}
	if _, err := Client().Get(srv.URL); err == nil {
		t.Fatal("self-signed server accepted without the bundle")
	}
	// With it the same request succeeds.
	cert := srv.Certificate()
	pem := "-----BEGIN CERTIFICATE-----\n" + base64Lines(cert.Raw) + "-----END CERTIFICATE-----\n"
	path := filepath.Join(t.TempDir(), "corp.pem")
	if err := os.WriteFile(path, []byte(pem), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Apply(core.Settings{CABundle: path}); err != nil {
		t.Fatal(err)
	}
	resp, err := Client().Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if tc := http.DefaultTransport.(*http.Transport).TLSClientConfig; tc == nil || tc.MinVersion != tls.VersionTLS12 {
		t.Fatal("TLS config not applied")
	}
}

func TestApplyErrors(t *testing.T) {
	restore(t)
	before := http.DefaultTransport
	cases := map[string]core.Settings{
		"missing bundle": {CABundle: filepath.Join(t.TempDir(), "nope.pem")},
		"not pem":        {CABundle: writeTemp(t, "hello")},
		"bad proxy":      {ProxyURL: "proxy.corp:3128"},
		"bad scheme":     {ProxyURL: "ftp://proxy.corp:3128"},
	}
	for name, s := range cases {
		if err := Apply(s); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if http.DefaultTransport != before {
		t.Fatal("a failed Apply replaced the transport")
	}
}

func TestApplyProxy(t *testing.T) {
	restore(t)
	if err := Apply(core.Settings{ProxyURL: "http://proxy.corp:3128"}); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, "https://example.com/", nil)
	u, err := http.DefaultTransport.(*http.Transport).Proxy(req)
	if err != nil || u == nil || u.Host != "proxy.corp:3128" {
		t.Fatalf("proxy = %v, %v", u, err)
	}
	if err := Apply(core.Settings{}); err != nil {
		t.Fatal(err)
	}
	if u, _ := http.DefaultTransport.(*http.Transport).Proxy(req); u != nil && strings.Contains(u.Host, "proxy.corp") {
		t.Fatal("clearing the setting kept the proxy")
	}
}

func writeTemp(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "f.pem")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func base64Lines(b []byte) string {
	enc := base64.StdEncoding.EncodeToString(b)
	var sb strings.Builder
	for len(enc) > 64 {
		sb.WriteString(enc[:64] + "\n")
		enc = enc[64:]
	}
	sb.WriteString(enc + "\n")
	return sb.String()
}

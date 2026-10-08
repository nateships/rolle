// Package netcfg applies the proxy and trust settings to every HTTP client.
// Go verifies TLS against the OS trust store (Keychain, CryptoAPI, /etc/ssl),
// so a corporate root installed there works without configuration. An extra
// PEM bundle and a fixed proxy cover the other cases.
package netcfg

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sync/atomic"

	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/debug"
)

var (
	base = http.DefaultTransport.(*http.Transport).Clone()
	// current is the transport Apply last built. Readers load it without a
	// lock, so http.DefaultTransport itself never changes after init.
	current atomic.Pointer[http.Transport]
)

func init() {
	// Every client in the process, the SDKs included, rides on the default
	// transport. Wrap it from the start so the host log misses nothing.
	current.Store(base.Clone())
	http.DefaultTransport = logged{}
}

// logged records the host of every request in the diagnostic log, so a user
// who runs with --debug sees each place the process talks to. Only the method
// and the host are logged: no path, no query, no header. Requests go to the
// current transport.
type logged struct{}

func (logged) RoundTrip(req *http.Request) (*http.Response, error) {
	debug.Logf("net", "%s %s", req.Method, req.URL.Host)
	return current.Load().RoundTrip(req)
}

// Apply rebuilds the current transport from the settings. An unreadable
// bundle or a malformed proxy URL is an error and leaves the transport as is.
// After the change, Apply closes the idle connections of the previous
// transport, because no new request uses that transport.
func Apply(s core.Settings) error {
	t := base.Clone()
	pool, err := roots(s.CABundle)
	if err != nil {
		return err
	}
	if pool != nil {
		t.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	}
	if s.ProxyURL != "" {
		u, err := url.Parse(s.ProxyURL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5") {
			return fmt.Errorf("proxy URL %q must look like http://host:port", s.ProxyURL)
		}
		t.Proxy = http.ProxyURL(u)
	}
	current.Swap(t).CloseIdleConnections()
	return nil
}

// Client returns a client on the default transport. Pass it to SDKs that
// build their own client, so they follow the same proxy and trust settings.
func Client() *http.Client {
	return &http.Client{Transport: http.DefaultTransport}
}

// roots returns the system roots plus the certificates in the bundle from the
// settings and in AWS_CA_BUNDLE, which the AWS tools use. It returns nil when
// no bundle is set. A bad AWS_CA_BUNDLE is logged and skipped, because the
// user did not set it in rolle.
func roots(setting string) (*x509.CertPool, error) {
	env := os.Getenv("AWS_CA_BUNDLE")
	if setting == "" && env == "" {
		return nil, nil
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if setting != "" {
		if err := appendPEM(pool, setting); err != nil {
			return nil, err
		}
	}
	if env != "" {
		if err := appendPEM(pool, env); err != nil {
			debug.Logf("net", "skip AWS_CA_BUNDLE: %v", err)
		}
	}
	return pool, nil
}

// appendPEM adds the certificates in the PEM file to pool.
func appendPEM(pool *x509.CertPool, path string) error {
	pem, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("CA bundle: %w", err)
	}
	if !pool.AppendCertsFromPEM(pem) {
		return fmt.Errorf("CA bundle %s holds no PEM certificates", path)
	}
	return nil
}

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
	"sync"

	"github.com/nateships/rolle/internal/core"
)

var (
	mu   sync.Mutex
	base = http.DefaultTransport.(*http.Transport).Clone()
)

// Apply rebuilds the default transport from the settings. An unreadable
// bundle or a malformed proxy URL is an error and leaves the transport as is.
func Apply(s core.Settings) error {
	t := base.Clone()
	if s.CABundle != "" {
		pool, err := poolWith(s.CABundle)
		if err != nil {
			return err
		}
		t.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	}
	if s.ProxyURL != "" {
		u, err := url.Parse(s.ProxyURL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5") {
			return fmt.Errorf("proxy URL %q must look like http://host:port", s.ProxyURL)
		}
		t.Proxy = http.ProxyURL(u)
	}
	mu.Lock()
	defer mu.Unlock()
	http.DefaultTransport = t
	return nil
}

// Client returns a client on the current default transport. Pass it to SDKs
// that build their own client, so they follow the same proxy and trust settings.
func Client() *http.Client {
	mu.Lock()
	defer mu.Unlock()
	return &http.Client{Transport: http.DefaultTransport}
}

// poolWith returns the system roots plus the certificates in the PEM file.
func poolWith(path string) (*x509.CertPool, error) {
	pem, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("CA bundle: %w", err)
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("CA bundle %s holds no PEM certificates", path)
	}
	return pool, nil
}

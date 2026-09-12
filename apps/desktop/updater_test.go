package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

func TestParsePublicKeyPEMAndBase64(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	pemText := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	got, err := parsePublicKey(pemText)
	if err != nil || string(got) != string(pub) {
		t.Fatalf("pem: %v", err)
	}
	got, err = parsePublicKey(base64.StdEncoding.EncodeToString(pub))
	if err != nil || string(got) != string(pub) {
		t.Fatalf("base64: %v", err)
	}
	if _, err := parsePublicKey(""); err == nil {
		t.Fatal("empty key accepted")
	}
	if _, err := parsePublicKey(updaterPublicKey); err != nil {
		t.Fatalf("embedded key: %v", err)
	}
}

func TestNewestManifestURLSkipsDraftsAndReleasesWithoutManifest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[
		 {"draft": true, "assets": [{"name": "manifest.json", "browser_download_url": "https://x/draft"}]},
		 {"draft": false, "prerelease": true, "assets": [{"name": "rolle.dmg", "browser_download_url": "https://x/dmg"}]},
		 {"draft": false, "prerelease": true, "assets": [{"name": "manifest.json", "browser_download_url": "https://x/rc/manifest.json"}]},
		 {"draft": false, "assets": [{"name": "manifest.json", "browser_download_url": "https://x/stable/manifest.json"}]}
		]`))
	}))
	defer srv.Close()
	got, err := newestManifestURL(context.Background(), srv.Client(), srv.URL)
	if err != nil || got != "https://x/rc/manifest.json" {
		t.Fatalf("got %q, %v", got, err)
	}
	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`[]`)) }))
	defer empty.Close()
	if _, err := newestManifestURL(context.Background(), empty.Client(), empty.URL); err == nil {
		t.Fatal("no releases accepted")
	}
}

func TestSignedRequiresASignature(t *testing.T) {
	if !signed(nil) {
		t.Fatal("no release is not a verification failure")
	}
	if signed(&updater.Release{Version: "1.0.0"}) {
		t.Fatal("a manifest without verification must be refused")
	}
	if signed(&updater.Release{Verification: &updater.Verification{Digest: []byte{1}}}) {
		t.Fatal("a digest-only manifest must be refused")
	}
	if !signed(&updater.Release{Verification: &updater.Verification{Signature: []byte{1}}}) {
		t.Fatal("a signed manifest must pass")
	}
}

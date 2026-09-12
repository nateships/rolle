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
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestAppBundleAndWritable(t *testing.T) {
	if got := appBundle("/Applications/rolle.app/Contents/MacOS/rolle"); got != "/Applications/rolle.app" {
		t.Fatalf("appBundle = %q", got)
	}
	if got := appBundle("/usr/local/bin/rolle-desktop"); got != "" {
		t.Fatalf("appBundle outside a bundle = %q", got)
	}
	dir := t.TempDir()
	if !writable(dir) {
		t.Fatal("a fresh temp dir must be writable")
	}
	if runtime.GOOS != "windows" && os.Getuid() != 0 {
		locked := filepath.Join(dir, "locked")
		if err := os.Mkdir(locked, 0o555); err != nil {
			t.Fatal(err)
		}
		if writable(locked) {
			t.Fatal("a read-only dir must not be writable")
		}
	}
}

func TestUpdateTargetAndWindowsScripts(t *testing.T) {
	dir := t.TempDir()
	if target, elevate := updateTarget("windows", filepath.Join(dir, "rolle.exe")); target != filepath.Join(dir, "rolle.exe") || elevate {
		t.Fatalf("writable dir = %q %v", target, elevate)
	}
	if target, elevate := updateTarget("linux", "/usr/local/bin/rolle-desktop"); target != "/usr/local/bin/rolle-desktop" || elevate {
		t.Fatalf("linux = %q %v", target, elevate)
	}
	if target, elevate := updateTarget("darwin", "/usr/local/bin/rolle-desktop"); target != "" || elevate {
		t.Fatalf("darwin outside a bundle = %q %v", target, elevate)
	}
	swap := windowsSwapCommand(`C:\Temp\wails-update-1\rolle.exe`, `C:\Program Files\nateships\rolle\rolle.exe`)
	if !strings.HasPrefix(swap, `/c move /y "C:\Program Files\nateships\rolle\rolle.exe" "C:\Program Files\nateships\rolle\rolle.exe.old.`) ||
		!strings.HasSuffix(swap, `" && copy /y "C:\Temp\wails-update-1\rolle.exe" "C:\Program Files\nateships\rolle\rolle.exe"`) {
		t.Fatalf("swap = %s", swap)
	}
	script := windowsElevateScript(`/c echo it's`)
	if !strings.Contains(script, `-ArgumentList '/c echo it''s' -Verb RunAs -Wait`) {
		t.Fatalf("elevate script = %s", script)
	}
	if got := windowsRelaunchScript(42, `C:\rolle.exe`); got != `Wait-Process -Id 42 -ErrorAction SilentlyContinue; Start-Process -FilePath 'C:\rolle.exe'` {
		t.Fatalf("relaunch script = %s", got)
	}
	exe := filepath.Join(dir, "rolle.exe")
	for _, n := range []string{exe + ".old.1", exe + ".old.2", filepath.Join(dir, "other.old.1")} {
		if err := os.WriteFile(n, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sweepAsides(exe)
	left, _ := filepath.Glob(filepath.Join(dir, "*"))
	if len(left) != 1 || filepath.Base(left[0]) != "other.old.1" {
		t.Fatalf("after sweep = %v", left)
	}
}

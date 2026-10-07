package main

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/updater"

	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/version"
)

// feedPub and feedPriv sign the manifests of the fake feed.
var feedPub, feedPriv, _ = ed25519.GenerateKey(rand.Reader)

// signManifest returns the .sig file for body, as `wails3 updater sign`
// makes it: Ed25519ph over the SHA-512 digest, in base64.
func signManifest(t *testing.T, body []byte) []byte {
	t.Helper()
	digest := sha512.Sum512(body)
	s, err := feedPriv.Sign(nil, digest[:], &ed25519.Options{Hash: crypto.SHA512})
	if err != nil {
		t.Fatal(err)
	}
	return []byte(base64.StdEncoding.EncodeToString(s) + "\n")
}

// updateFeed is a fake GitHub: a stable manifest at /manifest.json, a
// pre-release manifest at /beta/manifest.json, a releases API at /releases,
// an unsigned manifest at /unsigned/manifest.json, and one artifact. Each
// manifest has a signature at the same path plus .sig, except
// /nosig/manifest.json. The signature of /tampered/manifest.json is for
// other bytes.
func updateFeed(t *testing.T) *httptest.Server {
	t.Helper()
	sig := base64.StdEncoding.EncodeToString([]byte("signature"))
	manifest := func(ver string, signed bool) []byte {
		art := map[string]any{"url": "app.zip", "digestAlgo": "sha256", "digest": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))}
		if signed {
			art["signatureAlgo"], art["signature"] = "ed25519", sig
		}
		b, err := json.Marshal(map[string]any{"schemaVersion": 1, "version": ver, "notes": "notes for " + ver, "artifacts": []any{art}})
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	mux := http.NewServeMux()
	serve := func(path string, body, sig []byte) {
		mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) })
		if sig != nil {
			mux.HandleFunc(path+".sig", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(sig) })
		}
	}
	signed := func(path string, body []byte) { serve(path, body, signManifest(t, body)) }
	signed("/manifest.json", manifest("2.0.0", true))
	signed("/beta/manifest.json", manifest("2.1.0-rc.1", true))
	signed("/unsigned/manifest.json", manifest("3.0.0", false))
	signed("/elsewhere/manifest.json", bytes.Replace(manifest("2.0.0", true), []byte(`"app.zip"`), []byte(`"https://example.com/app.zip"`), 1))
	serve("/nosig/manifest.json", manifest("2.0.0", true), nil)
	serve("/tampered/manifest.json", manifest("9.0.0", true), signManifest(t, manifest("2.0.0", true)))
	// checkReq asks for darwin, so the windows-only manifest has nothing for it.
	signed("/windows-only/manifest.json", bytes.Replace(manifest("2.0.0", true), []byte(`"url":`), []byte(`"platform":"windows","arch":"amd64","url":`), 1))
	mux.HandleFunc("/app.zip", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("artifact bytes")) })
	mux.HandleFunc("/broken", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) })
	srv := httptest.NewServer(mux)
	mux.HandleFunc("/releases", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"draft": false, "prerelease": true, "assets": [{"name": "manifest.json", "browser_download_url": "` + srv.URL + `/beta/manifest.json"}]}]`))
	})
	t.Cleanup(srv.Close)
	return srv
}

// pointFeeds routes the updater at the fake feed for one test.
func pointFeeds(t *testing.T, manifest, api string) {
	t.Helper()
	oldManifest, oldAPI, oldPrefix := manifestURL, releasesAPI, artifactPrefix
	manifestURL, releasesAPI = manifest, api
	if u, err := url.Parse(manifest); err == nil && u.Host != "" {
		artifactPrefix = u.Scheme + "://" + u.Host + "/"
	}
	t.Cleanup(func() { manifestURL, releasesAPI, artifactPrefix = oldManifest, oldAPI, oldPrefix })
}

func settingsWith(channel string) func() (core.Settings, error) {
	return func() (core.Settings, error) { return core.Settings{UpdateChannel: channel}, nil }
}

var checkReq = updater.CheckRequest{CurrentVersion: "1.0.0", Platform: "darwin", Arch: "arm64"}

func TestChannelProviderStableFollowsLatestRelease(t *testing.T) {
	srv := updateFeed(t)
	pointFeeds(t, srv.URL+"/manifest.json", srv.URL+"/releases")
	p := &channelProvider{settings: settingsWith(""), key: feedPub}
	if p.Name() != "github" {
		t.Fatal(p.Name())
	}
	rel, err := p.Check(context.Background(), checkReq)
	if err != nil {
		t.Fatal(err)
	}
	if rel == nil || rel.Version != "2.0.0" || rel.Notes != "notes for 2.0.0" {
		t.Fatalf("release = %+v", rel)
	}
	var got bytes.Buffer
	if err := p.Download(context.Background(), rel, &got, nil); err != nil {
		t.Fatal(err)
	}
	if got.String() != "artifact bytes" {
		t.Fatalf("downloaded %q", got.String())
	}
}

func TestChannelProviderBetaFollowsNewestPreRelease(t *testing.T) {
	srv := updateFeed(t)
	pointFeeds(t, srv.URL+"/manifest.json", srv.URL+"/releases")
	p := &channelProvider{settings: settingsWith("beta"), key: feedPub}
	rel, err := p.Check(context.Background(), checkReq)
	if err != nil {
		t.Fatal(err)
	}
	if rel == nil || rel.Version != "2.1.0-rc.1" {
		t.Fatalf("release = %+v", rel)
	}
}

func TestChannelProviderBetaFallsBackToStableWhenAPIFails(t *testing.T) {
	srv := updateFeed(t)
	pointFeeds(t, srv.URL+"/manifest.json", srv.URL+"/broken")
	p := &channelProvider{settings: settingsWith("beta"), key: feedPub}
	rel, err := p.Check(context.Background(), checkReq)
	if err != nil {
		t.Fatal(err)
	}
	if rel == nil || rel.Version != "2.0.0" {
		t.Fatalf("release = %+v", rel)
	}
	// A settings read failure also means stable.
	p = &channelProvider{settings: func() (core.Settings, error) { return core.Settings{}, errors.New("boom") }, key: feedPub}
	if rel, err := p.Check(context.Background(), checkReq); err != nil || rel == nil || rel.Version != "2.0.0" {
		t.Fatalf("release = %+v, %v", rel, err)
	}
}

func TestChannelProviderRefusesUnsignedManifest(t *testing.T) {
	srv := updateFeed(t)
	pointFeeds(t, srv.URL+"/unsigned/manifest.json", srv.URL+"/releases")
	p := &channelProvider{settings: settingsWith(""), key: feedPub}
	if _, err := p.Check(context.Background(), checkReq); err == nil || !strings.Contains(err.Error(), "not signed") {
		t.Fatalf("err = %v", err)
	}
}

func TestChannelProviderVerifiesTheManifestSignature(t *testing.T) {
	srv := updateFeed(t)
	for path, want := range map[string]string{
		"/nosig/manifest.json":    "signature request failed: HTTP 404",
		"/tampered/manifest.json": "signature did not verify",
	} {
		pointFeeds(t, srv.URL+path, srv.URL+"/releases")
		p := &channelProvider{settings: settingsWith(""), key: feedPub}
		if _, err := p.Check(context.Background(), checkReq); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v", path, err)
		}
	}
	// Another key does not verify the stable manifest.
	pointFeeds(t, srv.URL+"/manifest.json", srv.URL+"/releases")
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	if _, err := (&channelProvider{settings: settingsWith(""), key: other}).Check(context.Background(), checkReq); err == nil {
		t.Fatal("manifest accepted with another key")
	}
	if _, err := (&channelProvider{settings: settingsWith("")}).Check(context.Background(), checkReq); err == nil {
		t.Fatal("manifest accepted with no key")
	}
	// No manifest at all means no update.
	pointFeeds(t, srv.URL+"/missing/manifest.json", srv.URL+"/releases")
	if rel, err := (&channelProvider{settings: settingsWith(""), key: feedPub}).Check(context.Background(), checkReq); err != nil || rel != nil {
		t.Fatalf("missing manifest = %+v, %v", rel, err)
	}
}

func TestManifestTransportServesOnlyTheManifest(t *testing.T) {
	srv := updateFeed(t)
	client, err := verifiedManifestClient(srv.URL+"/manifest.json", []byte(`{"verified":true}`))
	if err != nil {
		t.Fatal(err)
	}
	get := func(u string) string {
		resp, err := client.Get(u)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	if got := get(srv.URL + "/manifest.json?platform=darwin&arch=arm64"); got != `{"verified":true}` {
		t.Fatalf("manifest = %s", got)
	}
	if got := get(srv.URL + "/app.zip"); got != "artifact bytes" {
		t.Fatalf("artifact = %s", got)
	}
}

// The match runs on the error text of the Wails endpoint provider. A new
// wording in a Wails update makes this test fail.
func TestChannelProviderTreatsNoArtifactForThisPlatformAsNoUpdate(t *testing.T) {
	srv := updateFeed(t)
	pointFeeds(t, srv.URL+"/windows-only/manifest.json", srv.URL+"/releases")
	p := &channelProvider{settings: settingsWith(""), key: feedPub}
	if rel, err := p.Check(context.Background(), checkReq); err != nil || rel != nil {
		t.Fatalf("release = %+v, %v", rel, err)
	}
	req := checkReq
	req.Platform, req.Arch = "windows", "amd64"
	if rel, err := p.Check(context.Background(), req); err != nil || rel == nil {
		t.Fatalf("windows release = %+v, %v", rel, err)
	}
}

func TestPackageManaged(t *testing.T) {
	for _, c := range []struct {
		goos, appImage string
		want           bool
	}{
		{"linux", "", true},
		{"linux", "/home/me/rolle.AppImage", false},
		{"darwin", "", false},
		{"windows", "", false},
	} {
		if got := packageManaged(c.goos, c.appImage); got != c.want {
			t.Errorf("packageManaged(%q, %q) = %v", c.goos, c.appImage, got)
		}
	}
}

func TestChannelProviderRefusesArtifactOutsideReleases(t *testing.T) {
	srv := updateFeed(t)
	pointFeeds(t, srv.URL+"/elsewhere/manifest.json", srv.URL+"/releases")
	p := &channelProvider{settings: settingsWith(""), key: feedPub}
	if _, err := p.Check(context.Background(), checkReq); err == nil || !strings.Contains(err.Error(), "not a rolle release download") {
		t.Fatalf("err = %v", err)
	}
}

func TestTrustedArtifactURL(t *testing.T) {
	const prefix = "https://github.com/nateships/rolle/releases/download/"
	if artifactPrefix != prefix {
		t.Fatalf("artifactPrefix = %q", artifactPrefix)
	}
	for raw, want := range map[string]bool{
		prefix + "v1.2.3/rolle-update-macos-universal.zip":                       true,
		"http://github.com/nateships/rolle/releases/download/v1.2.3/a.zip":       false,
		"https://github.com.evil.example/nateships/rolle/releases/download/a":    false,
		"https://github.com/nateships/other/releases/download/v1.2.3/a.zip":      false,
		"https://user@github.com/nateships/rolle/releases/download/v1.2.3/a.zip": false,
		prefix + "v1.2.3/../../../../other/repo/a.zip":                           false,
		"": false,
	} {
		if got := trustedArtifactURL(raw, prefix); got != want {
			t.Errorf("trustedArtifactURL(%q) = %v, want %v", raw, got, want)
		}
	}
}

func TestChannelProviderUpToDateReturnsNoRelease(t *testing.T) {
	srv := updateFeed(t)
	pointFeeds(t, srv.URL+"/manifest.json", srv.URL+"/releases")
	p := &channelProvider{settings: settingsWith(""), key: feedPub}
	req := checkReq
	req.CurrentVersion = "2.0.0"
	rel, err := p.Check(context.Background(), req)
	if err != nil || rel != nil {
		t.Fatalf("release = %+v, %v", rel, err)
	}
}

func TestChannelProviderDownloadNeedsCheckFirst(t *testing.T) {
	p := &channelProvider{settings: settingsWith(""), key: feedPub}
	err := p.Download(context.Background(), &updater.Release{}, &bytes.Buffer{}, nil)
	if err == nil || !strings.Contains(err.Error(), "check before download") {
		t.Fatalf("err = %v", err)
	}
}

func TestChannelProviderRejectsEmptyManifestURL(t *testing.T) {
	pointFeeds(t, "", "")
	p := &channelProvider{settings: settingsWith(""), key: feedPub}
	if _, err := p.Check(context.Background(), checkReq); err == nil {
		t.Fatal("empty manifest URL accepted")
	}
}

func TestNewestManifestURLReportsHTTPErrors(t *testing.T) {
	srv := updateFeed(t)
	if _, err := newestManifestURL(context.Background(), srv.Client(), srv.URL+"/broken"); err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("err = %v", err)
	}
	if _, err := newestManifestURL(context.Background(), srv.Client(), "http://127.0.0.1:1/closed"); err == nil {
		t.Fatal("unreachable host accepted")
	}
	if _, err := newestManifestURL(context.Background(), srv.Client(), "::bad url"); err == nil {
		t.Fatal("malformed URL accepted")
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{not json`)) }))
	defer bad.Close()
	if _, err := newestManifestURL(context.Background(), bad.Client(), bad.URL); err == nil {
		t.Fatal("malformed JSON accepted")
	}
}

func TestPendingUpdateReportsLastBackgroundResult(t *testing.T) {
	pendingUpdate.mu.Lock()
	old := pendingUpdate.info
	pendingUpdate.info = nil
	pendingUpdate.mu.Unlock()
	t.Cleanup(func() {
		pendingUpdate.mu.Lock()
		pendingUpdate.info = old
		pendingUpdate.mu.Unlock()
	})
	r := &RolleService{}
	if r.PendingUpdate() != nil {
		t.Fatal("pending update before any check")
	}
	info := UpdateInfo{Available: true, Version: "2.0.0"}
	pendingUpdate.mu.Lock()
	pendingUpdate.info = &info
	pendingUpdate.mu.Unlock()
	if got := r.PendingUpdate(); got == nil || got.Version != "2.0.0" {
		t.Fatalf("pending = %+v", got)
	}
}

func TestBackgroundCheckHonoursAutoUpdateOff(t *testing.T) {
	r := testrolle(t)
	st, _ := r.Settings()
	st.AutoUpdateOff = true
	if _, err := r.UpdateSettings(st); err != nil {
		t.Fatal(err)
	}
	// No app is attached: reaching the updater would panic.
	backgroundCheck(nil, r.svc)
}

func TestSetupUpdaterSkipsDevBuilds(t *testing.T) {
	if !isDevBuild() {
		t.Skipf("release version %q baked in", version.Version)
	}
	if err := setupUpdater(nil, nil); err != nil {
		t.Fatal(err)
	}
}

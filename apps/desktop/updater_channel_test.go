package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/updater"

	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/version"
)

// updateFeed is a fake GitHub: a stable manifest at /manifest.json, a
// pre-release manifest at /beta/manifest.json, a releases API at /releases,
// an unsigned manifest at /unsigned/manifest.json, and one artifact.
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
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(manifest("2.0.0", true)) })
	mux.HandleFunc("/beta/manifest.json", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(manifest("2.1.0-rc.1", true)) })
	mux.HandleFunc("/unsigned/manifest.json", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(manifest("3.0.0", false)) })
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
	oldManifest, oldAPI := manifestURL, releasesAPI
	manifestURL, releasesAPI = manifest, api
	t.Cleanup(func() { manifestURL, releasesAPI = oldManifest, oldAPI })
}

func settingsWith(channel string) func() (core.Settings, error) {
	return func() (core.Settings, error) { return core.Settings{UpdateChannel: channel}, nil }
}

var checkReq = updater.CheckRequest{CurrentVersion: "1.0.0", Platform: "darwin", Arch: "arm64"}

func TestChannelProviderStableFollowsLatestRelease(t *testing.T) {
	srv := updateFeed(t)
	pointFeeds(t, srv.URL+"/manifest.json", srv.URL+"/releases")
	p := &channelProvider{settings: settingsWith("")}
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
	p := &channelProvider{settings: settingsWith("beta")}
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
	p := &channelProvider{settings: settingsWith("beta")}
	rel, err := p.Check(context.Background(), checkReq)
	if err != nil {
		t.Fatal(err)
	}
	if rel == nil || rel.Version != "2.0.0" {
		t.Fatalf("release = %+v", rel)
	}
	// A settings read failure also means stable.
	p = &channelProvider{settings: func() (core.Settings, error) { return core.Settings{}, errors.New("boom") }}
	if rel, err := p.Check(context.Background(), checkReq); err != nil || rel == nil || rel.Version != "2.0.0" {
		t.Fatalf("release = %+v, %v", rel, err)
	}
}

func TestChannelProviderRefusesUnsignedManifest(t *testing.T) {
	srv := updateFeed(t)
	pointFeeds(t, srv.URL+"/unsigned/manifest.json", srv.URL+"/releases")
	p := &channelProvider{settings: settingsWith("")}
	if _, err := p.Check(context.Background(), checkReq); err == nil || !strings.Contains(err.Error(), "not signed") {
		t.Fatalf("err = %v", err)
	}
}

func TestChannelProviderUpToDateReturnsNoRelease(t *testing.T) {
	srv := updateFeed(t)
	pointFeeds(t, srv.URL+"/manifest.json", srv.URL+"/releases")
	p := &channelProvider{settings: settingsWith("")}
	req := checkReq
	req.CurrentVersion = "2.0.0"
	rel, err := p.Check(context.Background(), req)
	if err != nil || rel != nil {
		t.Fatalf("release = %+v, %v", rel, err)
	}
}

func TestChannelProviderDownloadNeedsCheckFirst(t *testing.T) {
	p := &channelProvider{settings: settingsWith("")}
	err := p.Download(context.Background(), &updater.Release{}, &bytes.Buffer{}, nil)
	if err == nil || !strings.Contains(err.Error(), "check before download") {
		t.Fatalf("err = %v", err)
	}
}

func TestChannelProviderRejectsEmptyManifestURL(t *testing.T) {
	pointFeeds(t, "", "")
	p := &channelProvider{settings: settingsWith("")}
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

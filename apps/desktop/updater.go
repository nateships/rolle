package main

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/endpoint"

	"github.com/nateships/rolle/internal/app"
	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/debug"
	"github.com/nateships/rolle/internal/version"
)

// manifestURL is the signed update manifest of the latest stable release.
const manifestURL = "https://github.com/nateships/rolle/releases/latest/download/manifest.json"

// releasesAPI lists releases newest first, pre-releases included.
const releasesAPI = "https://api.github.com/repos/nateships/rolle/releases?per_page=10"

// checkInterval is how often background update checks run.
const checkInterval = 6 * time.Hour

// startupCheckDelay gives the window time to appear before the first check.
const startupCheckDelay = 20 * time.Second

// EventUpdateAvailable carries an UpdateInfo when a background check finds a
// newer release. The sidebar shows it; the updater window opens on request.
const EventUpdateAvailable = "update:available"

// updaterPublicKey is the Ed25519 trust root, generated with `wails3 updater genkey`.
// Release artifacts are signed with the matching private key in CI.
//
//go:embed build/updater.pub
var updaterPublicKey string

// isDevBuild reports whether this binary has no release version baked in.
func isDevBuild() bool {
	return version.Version == "" || strings.Contains(version.Version, "dev")
}

// setupUpdater configures self-updates. Dev builds skip it: there is nothing
// to compare against and no manifest to fetch. Background checks run on a
// ticker here, not in the Wails updater, so the AutoUpdateOff setting is read
// on every tick and a change applies without a restart.
func setupUpdater(a *application.App, svc *app.Service) error {
	if isDevBuild() {
		debug.Logf("updater", "dev build %q, updater disabled", version.Version)
		return nil
	}
	key, err := parsePublicKey(updaterPublicKey)
	if err != nil {
		return err
	}
	cfg := updater.Config{
		CurrentVersion: strings.TrimPrefix(version.Version, "v"),
		Providers:      []updater.Provider{&channelProvider{settings: svc.Settings}},
		PublicKey:      key,
	}
	if err := a.Updater.Init(cfg); err != nil {
		return err
	}
	debug.Logf("updater", "configured for %s, first check in %v, then every %v", cfg.CurrentVersion, startupCheckDelay, checkInterval)
	go func() {
		time.Sleep(startupCheckDelay)
		for {
			backgroundCheck(a, svc)
			time.Sleep(checkInterval)
		}
	}()
	return nil
}

// backgroundCheck asks the feed for a newer release and tells the frontend
// when there is one. It never opens the updater window on its own.
func backgroundCheck(a *application.App, svc *app.Service) {
	if st, err := svc.Settings(); err != nil || st.AutoUpdateOff {
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rel, err := a.Updater.Check(c)
	if err != nil {
		debug.Logf("updater", "background check: %v", err)
		return
	}
	if rel == nil {
		return
	}
	debug.Logf("updater", "background check: %s available", rel.Version)
	a.Event.Emit(EventUpdateAvailable, UpdateInfo{
		Enabled:        true,
		CurrentVersion: version.Version,
		Available:      true,
		Version:        rel.Version,
		Notes:          rel.Notes,
		State:          string(a.Updater.State()),
	})
}

// parsePublicKey accepts the PEM file written by `wails3 updater genkey` or
// the raw key as base64, and returns the 32 Ed25519 public key bytes.
func parsePublicKey(text string) ([]byte, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, errors.New("updater public key is empty; run wails3 updater genkey")
	}
	if block, _ := pem.Decode([]byte(text)); block != nil {
		pub, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		ed, ok := pub.(ed25519.PublicKey)
		if !ok {
			return nil, errors.New("updater public key is not Ed25519")
		}
		return []byte(ed), nil
	}
	raw, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		return nil, err
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, errors.New("updater public key has the wrong length")
	}
	return raw, nil
}

// UpdateInfo is what the settings screen shows after a check.
type UpdateInfo struct {
	Enabled        bool   `json:"enabled"`
	CurrentVersion string `json:"currentVersion"`
	Available      bool   `json:"available"`
	Version        string `json:"version,omitempty"`
	Notes          string `json:"notes,omitempty"`
	State          string `json:"state"`
}

// CheckForUpdates asks the release feed for a newer version.
func (r *RolleService) CheckForUpdates() (UpdateInfo, error) {
	info := UpdateInfo{Enabled: !isDevBuild(), CurrentVersion: version.Version}
	if r.app == nil || isDevBuild() {
		info.State = "disabled"
		return info, nil
	}
	c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rel, err := r.app.Updater.Check(c)
	info.State = string(r.app.Updater.State())
	if err != nil {
		return info, err
	}
	if rel != nil {
		info.Available, info.Version, info.Notes = true, rel.Version, rel.Notes
	}
	return info, nil
}

// InstallUpdate downloads, verifies, and installs the latest release, then
// prompts through the updater window to restart.
func (r *RolleService) InstallUpdate() error {
	if r.app == nil || isDevBuild() {
		return nil
	}
	// The updater window keeps this context for its Install and Retry
	// buttons, so it must outlive the call.
	return r.app.Updater.CheckAndInstall(context.Background())
}

// channelProvider reads the update channel from settings on every check, so
// a change applies without a restart. Stable follows the latest release.
// Beta follows the newest release of any kind, pre-releases included.
type channelProvider struct {
	settings func() (core.Settings, error)
	mu       sync.Mutex
	current  *endpoint.Provider
}

func (p *channelProvider) Name() string { return "github" }

func (p *channelProvider) Check(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	url := manifestURL
	if st, err := p.settings(); err == nil && st.UpdateChannel == "beta" {
		if u, err := newestManifestURL(ctx, http.DefaultClient, releasesAPI); err == nil {
			url = u
		} else {
			debug.Logf("updater", "beta channel: %v; using stable", err)
		}
	}
	ep, err := endpoint.New(endpoint.Config{URL: url})
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	p.current = ep
	p.mu.Unlock()
	rel, err := ep.Check(ctx, req)
	if err != nil {
		return nil, err
	}
	if !signed(rel) {
		return nil, errors.New("updater: release manifest is not signed")
	}
	return rel, nil
}

// signed reports whether a release carries a signature. A pinned public key
// alone does not demand one: the updater accepts a digest-only manifest.
func signed(rel *updater.Release) bool {
	return rel == nil || (rel.Verification != nil && len(rel.Verification.Signature) > 0)
}

func (p *channelProvider) Download(ctx context.Context, r *updater.Release, dst io.Writer, onProgress func(written, total int64)) error {
	p.mu.Lock()
	ep := p.current
	p.mu.Unlock()
	if ep == nil {
		return errors.New("updater: check before download")
	}
	return ep.Download(ctx, r, dst, onProgress)
}

// newestManifestURL returns the manifest.json asset of the newest published
// release, pre-release or not.
func newestManifestURL(ctx context.Context, client *http.Client, api string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("releases API: %s", resp.Status)
	}
	var releases []struct {
		Draft  bool `json:"draft"`
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&releases); err != nil {
		return "", err
	}
	for _, rel := range releases {
		if rel.Draft {
			continue
		}
		for _, a := range rel.Assets {
			if a.Name == "manifest.json" {
				return a.URL, nil
			}
		}
	}
	return "", errors.New("no release with a manifest")
}

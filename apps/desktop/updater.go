package main

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	_ "embed"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/endpoint"

	"github.com/nateships/rolle/internal/app"
	"github.com/nateships/rolle/internal/debug"
	"github.com/nateships/rolle/internal/version"
)

// manifestURL is the signed update manifest published with every release.
const manifestURL = "https://github.com/nateships/rolle/releases/latest/download/manifest.json"

// checkInterval is how often background update checks run.
const checkInterval = 6 * time.Hour

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
// to compare against and no manifest to fetch.
func setupUpdater(a *application.App, svc *app.Service) error {
	if isDevBuild() {
		debug.Logf("updater", "dev build %q, updater disabled", version.Version)
		return nil
	}
	key, err := parsePublicKey(updaterPublicKey)
	if err != nil {
		return err
	}
	provider, err := endpoint.New(endpoint.Config{URL: manifestURL})
	if err != nil {
		return err
	}
	interval := checkInterval
	if st, err := svc.Settings(); err == nil && st.AutoUpdateOff {
		interval = 0
	}
	cfg := updater.Config{
		CurrentVersion: strings.TrimPrefix(version.Version, "v"),
		Providers:      []updater.Provider{provider},
		PublicKey:      key,
		CheckInterval:  interval,
	}
	debug.Logf("updater", "configured for %s, background checks every %v", cfg.CurrentVersion, interval)
	return a.Updater.Init(cfg)
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
	c, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	return r.app.Updater.CheckAndInstall(c)
}

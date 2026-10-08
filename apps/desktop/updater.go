package main

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/sha512"
	"crypto/x509"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/endpoint"
	"golang.org/x/mod/semver"

	"github.com/nateships/rolle/internal/app"
	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/debug"
	"github.com/nateships/rolle/internal/version"
)

// manifestURL is the signed update manifest of the latest stable release.
var manifestURL = "https://github.com/nateships/rolle/releases/latest/download/manifest.json"

// releasesAPI lists releases newest first, pre-releases included.
var releasesAPI = "https://api.github.com/repos/nateships/rolle/releases?per_page=10"

// checkInterval is how often background update checks run.
const checkInterval = 6 * time.Hour

// EventUpdateAvailable carries an UpdateInfo when a background check finds a
// newer release. The sidebar shows it; the updater window opens on request.
const EventUpdateAvailable = "update:available"

// pendingUpdate remembers the last release a background check found. The
// first check runs at launch, often before the frontend listens for the
// event, so the frontend asks for this on mount.
var pendingUpdate struct {
	mu   sync.Mutex
	info *UpdateInfo
}

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
	if packageManaged(runtime.GOOS, os.Getenv("APPIMAGE")) {
		debug.Logf("updater", "installed by a package manager, updater disabled")
		return nil
	}
	key, err := parsePublicKey(updaterPublicKey)
	if err != nil {
		return err
	}
	cfg := updater.Config{
		CurrentVersion: strings.TrimPrefix(version.Version, "v"),
		Providers:      []updater.Provider{&channelProvider{settings: svc.Settings, key: key, store: svc}},
		PublicKey:      key,
	}
	if err := a.Updater.Init(cfg); err != nil {
		return err
	}
	debug.Logf("updater", "configured for %s, checking now and every %v", cfg.CurrentVersion, checkInterval)
	go func() {
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
	info := UpdateInfo{
		Enabled:        true,
		CurrentVersion: version.Version,
		Available:      true,
		Version:        rel.Version,
		Notes:          rel.Notes,
		State:          string(a.Updater.State()),
	}
	pendingUpdate.mu.Lock()
	pendingUpdate.info = &info
	pendingUpdate.mu.Unlock()
	a.Event.Emit(EventUpdateAvailable, info)
}

// PendingUpdate returns the release the last background check found, or nil.
// The frontend calls it on mount to catch a check that ran before it loaded.
func (r *RolleService) PendingUpdate() *UpdateInfo {
	pendingUpdate.mu.Lock()
	defer pendingUpdate.mu.Unlock()
	return pendingUpdate.info
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

// statePackageManager is the UpdateInfo state of an install that a package
// manager owns. The frontend then tells the user to update through it.
const statePackageManager = "package-manager"

// packageManaged reports whether a package manager owns the install. On
// Linux the updater replaces only an AppImage. A .deb install has no
// APPIMAGE variable, and its root-owned executable is the package
// manager's to replace.
func packageManaged(goos, appImage string) bool { return goos == "linux" && appImage == "" }

// CheckForUpdates asks the release feed for a newer version.
func (r *RolleService) CheckForUpdates() (UpdateInfo, error) {
	info := UpdateInfo{Enabled: !isDevBuild(), CurrentVersion: version.Version}
	if r.app == nil || isDevBuild() {
		info.State = "disabled"
		return info, nil
	}
	if packageManaged(runtime.GOOS, os.Getenv("APPIMAGE")) {
		info.Enabled, info.State = false, statePackageManager
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
	if packageManaged(runtime.GOOS, os.Getenv("APPIMAGE")) {
		return errors.New("update rolle with your package manager")
	}
	// The updater's helper replaces the app in place, which needs write access
	// to its folder. A standard macOS account has none in /Applications, a
	// standard Windows account none in Program Files. Those installs swap
	// through the administrator prompt instead.
	exe, _ := os.Executable()
	appImage := os.Getenv("APPIMAGE")
	target, elevate := updateTarget(runtime.GOOS, exe, appImage)
	if elevate {
		return r.installStaged(target, elevatedSwap)
	}
	// The AppImage runtime mounts the image read-only, so the updater's
	// helper cannot swap the executable it sees. The image file itself is
	// the user's and is replaced here.
	if target == appImage && appImage != "" {
		return r.installStaged(target, replaceFile)
	}
	// The updater window keeps this context for its Install and Retry
	// buttons, so it must outlive the call.
	return r.app.Updater.CheckAndInstall(context.Background())
}

// installStaged downloads and verifies the release like the updater does,
// then puts the staged file in place of target through swap and relaunches.
func (r *RolleService) installStaged(target string, swap func(staged, target string) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	u := r.app.Updater
	rel, err := u.Check(ctx)
	if err != nil {
		return err
	}
	if rel == nil {
		return errors.New("no update available")
	}
	if err := u.DownloadAndInstall(ctx); err != nil {
		return err
	}
	staged := u.DownloadedPath()
	if staged == "" {
		return errors.New("update: nothing staged")
	}
	// The manifest signature covers the artifact bytes but not the version
	// or the platform. Check them on the staged file.
	if err := checkStaged(runtime.GOOS, staged, rel.Version); err != nil {
		return err
	}
	debug.Logf("updater", "replacing %s from %s", target, staged)
	if err := swap(staged, target); err != nil {
		return err
	}
	// The swap copied the staged file, so the staging directory is now
	// garbage. The updater's helper removes it on the normal path.
	if dir := filepath.Dir(staged); strings.HasPrefix(filepath.Base(dir), "wails-update-") {
		_ = os.RemoveAll(dir)
	}
	if err := relaunchAfterExit(target); err != nil {
		debug.Logf("updater", "relaunch: %v", err)
	}
	r.app.Quit()
	return nil
}

// appBundle maps the executable inside a macOS bundle to the bundle path.
// Empty when exe is not inside a .app bundle.
func appBundle(exe string) string {
	const marker = ".app/Contents/MacOS/"
	i := strings.Index(exe, marker)
	if i < 0 {
		return ""
	}
	return exe[:i] + ".app"
}

// channelProvider reads the update channel from settings on every check, so
// a change applies without a restart. Stable follows the latest release.
// Beta follows the newest release of any kind, pre-releases included.
type channelProvider struct {
	settings func() (core.Settings, error)
	// key verifies the detached manifest signature.
	key ed25519.PublicKey
	// store keeps the highest manifest version of each feed. Nil keeps no
	// record.
	store   workspaceStore
	mu      sync.Mutex
	current *endpoint.Provider
}

// workspaceStore reads and writes the workspace file.
type workspaceStore interface {
	Load() (*core.Workspace, error)
	Save(w *core.Workspace) error
}

// errStaleManifest shows that a manifest with a valid signature is older than
// the installed version or older than a manifest seen before.
var errStaleManifest = errors.New("updater: the release feed serves an older version than expected, possibly a replayed manifest")

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
	// The artifact signature covers only the artifact. The manifest has its
	// own signature, which covers the version, the notes, and the URLs.
	body, err := fetchSignedManifest(ctx, url, p.key)
	if err != nil {
		return nil, err
	}
	if body == nil {
		return nil, nil
	}
	feed := "beta"
	if url == manifestURL {
		feed = "stable"
	}
	if err := p.checkFresh(feed, body, req.CurrentVersion); err != nil {
		return nil, err
	}
	client, err := verifiedManifestClient(url, body)
	if err != nil {
		return nil, err
	}
	ep, err := endpoint.New(endpoint.Config{URL: url, HTTPClient: client})
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	p.current = ep
	p.mu.Unlock()
	rel, err := ep.Check(ctx, req)
	if err != nil {
		// A release with no file for this platform is no update for it.
		if strings.Contains(err.Error(), "has no artifact for") {
			debug.Logf("updater", "%v", err)
			return nil, nil
		}
		return nil, err
	}
	if !signed(rel) {
		return nil, errors.New("updater: release manifest is not signed")
	}
	if rel != nil {
		if u, _ := rel.Metadata["endpoint.artifact.url"].(string); !trustedArtifactURL(u, artifactPrefix) {
			return nil, fmt.Errorf("updater: artifact URL %q is not a rolle release download", u)
		}
	}
	return rel, nil
}

// checkFresh refuses a verified manifest that is older than the installed
// version, or older than a manifest that this install verified before on the
// same feed. A user with edit rights on a release, but without the key, can
// upload an older manifest and its valid signature again. Without this
// check, the updater then reports "up to date" and updates stop.
func (p *channelProvider) checkFresh(feed string, body []byte, installed string) error {
	var m struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &m); err != nil {
		return fmt.Errorf("updater: decode manifest: %w", err)
	}
	got, have := semverOf(m.Version), semverOf(installed)
	// The stable feed can be older than a pre-release from the beta feed.
	preRelease := feed == "stable" && semver.Prerelease(have) != ""
	if !preRelease && semver.Compare(got, have) < 0 {
		return fmt.Errorf("%w (version %s, installed %s)", errStaleManifest, m.Version, installed)
	}
	if p.store == nil {
		return nil
	}
	// A failed read or write of the record does not stop the check.
	w, err := p.store.Load()
	if err != nil {
		debug.Logf("updater", "read seen manifest versions: %v", err)
		return nil
	}
	seen := w.UpdateSeen[feed]
	switch c := semver.Compare(got, semverOf(seen)); {
	case c < 0:
		return fmt.Errorf("%w (version %s, seen %s)", errStaleManifest, m.Version, seen)
	case c > 0:
		if w.UpdateSeen == nil {
			w.UpdateSeen = map[string]string{}
		}
		w.UpdateSeen[feed] = m.Version
		if err := p.store.Save(w); err != nil {
			debug.Logf("updater", "record seen manifest version: %v", err)
		}
	}
	return nil
}

// semverOf returns v with one leading "v", the form that x/mod/semver uses.
func semverOf(v string) string { return "v" + strings.TrimPrefix(v, "v") }

// artifactPrefix is the start of every release artifact URL. The updater
// refuses to download an artifact from a different location.
var artifactPrefix = "https://github.com/nateships/rolle/releases/download/"

// trustedArtifactURL reports whether raw starts with prefix and has no user
// information and no ".." path segment.
func trustedArtifactURL(raw, prefix string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || !strings.HasPrefix(raw, prefix) {
		return false
	}
	return !slices.Contains(strings.Split(u.Path, "/"), "..")
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

// manifestClient fetches the manifest and its signature.
var manifestClient = &http.Client{Timeout: 30 * time.Second}

// fetchSignedManifest downloads the manifest at raw and its signature at
// raw + ".sig". The signature is an Ed25519ph signature over the SHA-512
// digest of the manifest bytes, in base64, as `wails3 updater sign` writes
// it. The function returns nil and no error when the manifest does not
// exist. A missing or bad signature is an error.
func fetchSignedManifest(ctx context.Context, raw string, key ed25519.PublicKey) ([]byte, error) {
	if len(key) != ed25519.PublicKeySize {
		return nil, errors.New("updater: no manifest public key")
	}
	body, status, err := httpGet(ctx, raw, 8<<20)
	if err != nil {
		return nil, fmt.Errorf("updater: fetch manifest: %w", err)
	}
	if status == http.StatusNotFound {
		return nil, nil
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("updater: manifest request failed: HTTP %d", status)
	}
	sig, status, err := httpGet(ctx, raw+".sig", 1<<10)
	if err != nil {
		return nil, fmt.Errorf("updater: fetch manifest signature: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("updater: manifest signature request failed: HTTP %d", status)
	}
	s, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
	if err != nil {
		return nil, errors.New("updater: manifest signature is not base64")
	}
	digest := sha512.Sum512(body)
	if err := ed25519.VerifyWithOptions(key, digest[:], s, &ed25519.Options{Hash: crypto.SHA512}); err != nil {
		return nil, errors.New("updater: manifest signature did not verify")
	}
	return body, nil
}

// httpGet returns at most limit bytes of the body at raw and the status.
func httpGet(ctx context.Context, raw string, limit int64) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := manifestClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	return body, resp.StatusCode, err
}

// verifiedManifestClient is the HTTP client for the endpoint provider. A
// request for the manifest at raw gets body, which fetchSignedManifest
// verified. Thus the provider reads the same bytes that the signature
// covers. Other requests, such as the artifact download, go to the network.
func verifiedManifestClient(raw string, body []byte) (*http.Client, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	return &http.Client{Timeout: 30 * time.Second, Transport: manifestTransport{manifest: u, body: body, next: http.DefaultTransport}}, nil
}

type manifestTransport struct {
	manifest *url.URL
	body     []byte
	next     http.RoundTripper
}

// RoundTrip compares scheme, host, and path only. The endpoint provider
// adds query parameters to the manifest URL.
func (t manifestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != t.manifest.Scheme || !strings.EqualFold(req.URL.Host, t.manifest.Host) || req.URL.Path != t.manifest.Path {
		return t.next.RoundTrip(req)
	}
	return &http.Response{
		Status:        "200 OK",
		StatusCode:    http.StatusOK,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Type": {"application/json"}},
		Body:          io.NopCloser(bytes.NewReader(t.body)),
		ContentLength: int64(len(t.body)),
		Request:       req,
	}, nil
}

// replaceFile puts staged in place of target as an executable. It writes
// next to target first, so a failed copy changes nothing, then renames over
// target; a running AppImage keeps its mount through the rename.
func replaceFile(staged, target string) error {
	data, err := os.ReadFile(staged)
	if err != nil {
		return err
	}
	tmp := target + ".new"
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

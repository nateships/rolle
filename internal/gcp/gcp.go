// Package gcp implements service account impersonation on top of the user's
// Application Default Credentials created by gcloud.
package gcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/nateships/rolle/internal/core"
)

// ErrNoADC is returned when gcloud has not created Application Default Credentials.
var ErrNoADC = errors.New("gcp: no application default credentials; run `gcloud auth application-default login`")

// LoginCommand is what the user runs to create Application Default Credentials.
const LoginCommand = "gcloud auth application-default login"

// ADCPath returns the gcloud Application Default Credentials file location.
func ADCPath() (string, error) {
	if p := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"); p != "" {
		return p, nil
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("APPDATA"), "gcloud", "application_default_credentials.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "gcloud", "application_default_credentials.json"), nil
}

// adc is the subset of the ADC file Rolle reads.
type adc struct {
	Type         string `json:"type"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
	Account      string `json:"account,omitempty"`
}

// Account reports which Google account the ADC file belongs to, when known.
type Account struct {
	Email string
}

// DetectAccount reads the ADC file and returns its identity.
func DetectAccount(ctx context.Context) (Account, error) {
	path, err := ADCPath()
	if err != nil {
		return Account{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Account{}, ErrNoADC
	}
	if err != nil {
		return Account{}, err
	}
	var a adc
	if err := json.Unmarshal(data, &a); err != nil {
		return Account{}, fmt.Errorf("parse %s: %w", path, err)
	}
	acct := Account{Email: a.Account}
	if acct.Email == "" {
		// The lookup is cosmetic. Bound it so an offline machine does not
		// stall the caller for its whole deadline.
		lookup, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if email, err := userEmail(lookup, path, data); err == nil {
			acct.Email = email
		}
	}
	return acct, nil
}

// tokenSource builds a refreshing token source from an authorized_user ADC file.
// Only that type is accepted; service account keys and external accounts are
// rejected so a stray file cannot be used by mistake.
func tokenSource(ctx context.Context, path string, adcData []byte, scopes ...string) (oauth2.TokenSource, error) {
	var a adc
	if err := json.Unmarshal(adcData, &a); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if a.Type != "authorized_user" || a.RefreshToken == "" {
		return nil, fmt.Errorf("%s: expected authorized_user credentials from `%s`, found %q", path, LoginCommand, a.Type)
	}
	cfg := &oauth2.Config{ClientID: a.ClientID, ClientSecret: a.ClientSecret, Endpoint: google.Endpoint, Scopes: scopes}
	return cfg.TokenSource(ctx, &oauth2.Token{RefreshToken: a.RefreshToken}), nil
}

func userEmail(ctx context.Context, path string, adcData []byte) (string, error) {
	ts, err := tokenSource(ctx, path, adcData, "https://www.googleapis.com/auth/userinfo.email")
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://openidconnect.googleapis.com/v1/userinfo", nil)
	if err != nil {
		return "", err
	}
	resp, err := oauth2.NewClient(ctx, ts).Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	var info struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", err
	}
	return info.Email, nil
}

// SourceToken returns an access token for the signed-in user.
func SourceToken(ctx context.Context) (*oauth2.Token, error) {
	path, err := ADCPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoADC
	}
	if err != nil {
		return nil, err
	}
	ts, err := tokenSource(ctx, path, data, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, err
	}
	return ts.Token()
}

// Project is a GCP project visible to the user.
type Project struct {
	ID   string `json:"projectId"`
	Name string `json:"name"`
}

// ListProjects returns active projects the user can see.
func ListProjects(ctx context.Context, client *http.Client, token string) ([]Project, error) {
	if client == nil {
		client = http.DefaultClient
	}
	var out []Project
	url := "https://cloudresourcemanager.googleapis.com/v1/projects?filter=lifecycleState:ACTIVE"
	for url != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("list projects: %s: %s", resp.Status, body)
		}
		var page struct {
			Projects      []Project `json:"projects"`
			NextPageToken string    `json:"nextPageToken"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, err
		}
		out = append(out, page.Projects...)
		url = ""
		if page.NextPageToken != "" {
			url = "https://cloudresourcemanager.googleapis.com/v1/projects?filter=lifecycleState:ACTIVE&pageToken=" + page.NextPageToken
		}
	}
	return out, nil
}

// Impersonate mints a short-lived access token for a service account.
func Impersonate(ctx context.Context, client *http.Client, sourceToken, serviceAccount string, lifetime time.Duration) (core.Credentials, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if lifetime <= 0 {
		lifetime = time.Hour
	}
	body, err := json.Marshal(map[string]any{
		"scope":    []string{"https://www.googleapis.com/auth/cloud-platform"},
		"lifetime": fmt.Sprintf("%ds", int(lifetime/time.Second)),
	})
	if err != nil {
		return core.Credentials{}, err
	}
	url := fmt.Sprintf("https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/%s:generateAccessToken", serviceAccount)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return core.Credentials{}, err
	}
	req.Header.Set("Authorization", "Bearer "+sourceToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return core.Credentials{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return core.Credentials{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return core.Credentials{}, fmt.Errorf("impersonate %s: %s: %s", serviceAccount, resp.Status, data)
	}
	var out struct {
		AccessToken string    `json:"accessToken"`
		ExpireTime  time.Time `json:"expireTime"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return core.Credentials{}, err
	}
	exp := out.ExpireTime.UTC()
	return core.Credentials{Token: out.AccessToken, Expiration: &exp}, nil
}

// ConsoleURL links to the console for a project.
func ConsoleURL(projectID string) string {
	return "https://console.cloud.google.com/home/dashboard?project=" + projectID
}

// WriteImpersonatedADC writes an Application Default Credentials file that
// makes Google SDKs impersonate serviceAccount using the user's gcloud
// credentials as the source. The file inherits the source refresh token, so
// it is written with owner-only permissions next to the credential cache.
func WriteImpersonatedADC(path, serviceAccount string) error {
	src, err := ADCPath()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(src)
	if errors.Is(err, os.ErrNotExist) {
		return ErrNoADC
	}
	if err != nil {
		return err
	}
	var source map[string]any
	if err := json.Unmarshal(data, &source); err != nil {
		return fmt.Errorf("parse %s: %w", src, err)
	}
	if source["type"] != "authorized_user" {
		return fmt.Errorf("%s: expected authorized_user credentials, found %v", src, source["type"])
	}
	out, err := json.MarshalIndent(map[string]any{
		"type":                              "impersonated_service_account",
		"source_credentials":                source,
		"service_account_impersonation_url": fmt.Sprintf("https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/%s:generateAccessToken", serviceAccount),
		"delegates":                         []string{},
	}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o600)
}

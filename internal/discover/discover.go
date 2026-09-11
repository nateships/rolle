// Package discover finds cloud identities already configured on this machine
// by other tools, so onboarding can import them instead of asking again.
package discover

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/ini.v1"

	"github.com/nateships/rolle/internal/awsconfig"
	"github.com/nateships/rolle/internal/gcp"
)

// AWSPortal is an IAM Identity Center portal referenced by the AWS CLI config.
type AWSPortal struct {
	Alias    string   `json:"alias"`
	StartURL string   `json:"startUrl"`
	Region   string   `json:"region"`
	Profiles []string `json:"profiles"`
	// HasToken means the AWS CLI holds a valid SSO token for this portal.
	HasToken bool `json:"hasToken"`
}

// AzureTenant is a directory the az CLI has signed in to.
type AzureTenant struct {
	TenantID string `json:"tenantId"`
	Account  string `json:"account"`
}

// GCPAccount is the gcloud Application Default Credentials identity.
type GCPAccount struct {
	Account string `json:"account"`
}

// Result is everything found on the machine.
type Result struct {
	AWSPortals   []AWSPortal   `json:"awsPortals"`
	AzureTenants []AzureTenant `json:"azureTenants"`
	GCP          *GCPAccount   `json:"gcp,omitempty"`
}

// Scan inspects the AWS CLI config and SSO cache, the az CLI profile, and
// gcloud credentials. Missing tools are simply absent from the result.
func Scan(ctx context.Context) Result {
	var r Result
	if path, err := awsconfig.DefaultPath(); err == nil {
		r.AWSPortals = awsPortals(path, ssoCacheDir(path))
	}
	r.AzureTenants = azureTenants(azureProfilePath())
	if acct, err := gcp.DetectAccount(ctx); err == nil {
		r.GCP = &GCPAccount{Account: acct.Email}
	}
	return r
}

// awsPortals groups sso-session blocks and legacy sso profiles by start URL.
func awsPortals(configPath, cacheDir string) []AWSPortal {
	f, err := ini.LoadSources(ini.LoadOptions{SkipUnrecognizableLines: true}, configPath)
	if err != nil {
		return nil
	}
	byURL := map[string]*AWSPortal{}
	sessions := map[string]*AWSPortal{}
	add := func(startURL, region, alias string) *AWSPortal {
		startURL = strings.TrimSpace(startURL)
		if startURL == "" {
			return nil
		}
		p, ok := byURL[startURL]
		if !ok {
			p = &AWSPortal{StartURL: startURL, Region: region, Alias: alias}
			byURL[startURL] = p
		}
		if p.Region == "" {
			p.Region = region
		}
		if p.Alias == "" {
			p.Alias = alias
		}
		return p
	}
	for _, sec := range f.Sections() {
		name := sec.Name()
		switch {
		case strings.HasPrefix(name, "sso-session "):
			alias := strings.TrimPrefix(name, "sso-session ")
			if p := add(sec.Key("sso_start_url").String(), sec.Key("sso_region").String(), alias); p != nil {
				sessions[alias] = p
			}
		}
	}
	for _, sec := range f.Sections() {
		name := sec.Name()
		profile := ""
		switch {
		case name == "default":
			profile = "default"
		case strings.HasPrefix(name, "profile "):
			profile = strings.TrimPrefix(name, "profile ")
		default:
			continue
		}
		var p *AWSPortal
		if ref := sec.Key("sso_session").String(); ref != "" {
			p = sessions[ref]
		} else if u := sec.Key("sso_start_url").String(); u != "" {
			p = add(u, sec.Key("sso_region").String(), aliasFromURL(u))
		}
		if p != nil {
			p.Profiles = append(p.Profiles, profile)
		}
	}
	out := make([]AWSPortal, 0, len(byURL))
	for _, p := range byURL {
		if p.Alias == "" {
			p.Alias = aliasFromURL(p.StartURL)
		}
		if p.Region == "" {
			p.Region = "us-east-1"
		}
		sort.Strings(p.Profiles)
		_, p.HasToken = CLIToken(cacheDir, p.StartURL)
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Alias < out[j].Alias })
	return out
}

// aliasFromURL turns https://acme.awsapps.com/start into "acme".
func aliasFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "aws"
	}
	host := u.Host
	if i := strings.Index(host, "."); i > 0 {
		host = host[:i]
	}
	if host == "" || strings.HasPrefix(host, "d-") {
		return "aws"
	}
	return host
}

func ssoCacheDir(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "sso", "cache")
}

// Token is the AWS CLI's cached Identity Center token.
type Token struct {
	StartURL     string    `json:"startUrl"`
	Region       string    `json:"region"`
	AccessToken  string    `json:"accessToken"`
	ExpiresAt    time.Time `json:"expiresAt"`
	ClientID     string    `json:"clientId"`
	ClientSecret string    `json:"clientSecret"`
	RefreshToken string    `json:"refreshToken"`
}

// CLIToken finds a still-valid AWS CLI SSO token for startURL.
func CLIToken(cacheDir, startURL string) (Token, bool) {
	files, err := filepath.Glob(filepath.Join(cacheDir, "*.json"))
	if err != nil {
		return Token{}, false
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var t Token
		if json.Unmarshal(data, &t) != nil || t.AccessToken == "" {
			continue
		}
		if strings.TrimRight(t.StartURL, "/") != strings.TrimRight(startURL, "/") {
			continue
		}
		if !t.ExpiresAt.After(time.Now().Add(2 * time.Minute)) {
			continue
		}
		return t, true
	}
	return Token{}, false
}

// AWSCLITokenFor is CLIToken against the default AWS CLI cache location.
func AWSCLITokenFor(startURL string) (Token, bool) {
	path, err := awsconfig.DefaultPath()
	if err != nil {
		return Token{}, false
	}
	return CLIToken(ssoCacheDir(path), startURL)
}

func azureProfilePath() string {
	if d := os.Getenv("AZURE_CONFIG_DIR"); d != "" {
		return filepath.Join(d, "azureProfile.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".azure", "azureProfile.json")
}

// azureTenants lists distinct tenants from the az CLI profile.
func azureTenants(path string) []AzureTenant {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) || err != nil {
		return nil
	}
	data = []byte(strings.TrimPrefix(string(data), "\uFEFF")) // az writes a BOM
	var profile struct {
		Subscriptions []struct {
			TenantID string `json:"tenantId"`
			User     struct {
				Name string `json:"name"`
			} `json:"user"`
		} `json:"subscriptions"`
	}
	if json.Unmarshal(data, &profile) != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []AzureTenant
	for _, sub := range profile.Subscriptions {
		if sub.TenantID == "" || seen[sub.TenantID] {
			continue
		}
		seen[sub.TenantID] = true
		out = append(out, AzureTenant{TenantID: sub.TenantID, Account: sub.User.Name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TenantID < out[j].TenantID })
	return out
}

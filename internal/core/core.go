// Package core defines the domain model shared by the CLI and desktop app:
// integrations (identity sources), sessions (a role or identity you can assume),
// and the credentials a session yields.
package core

import (
	"errors"
	"strings"
	"time"
)

// Cloud identifies a cloud provider.
type Cloud string

const (
	CloudAWS   Cloud = "aws"
	CloudAzure Cloud = "azure"
	CloudGCP   Cloud = "gcp"
)

// Kind identifies how a session obtains credentials.
type Kind string

const (
	// KindAWSIAMUser uses long-lived access keys stored in the secret store.
	KindAWSIAMUser Kind = "aws-iam-user"
	// KindAWSAssumeRole calls STS AssumeRole from another AWS session.
	KindAWSAssumeRole Kind = "aws-assume-role"
	// KindAWSSSORole fetches role credentials from IAM Identity Center.
	KindAWSSSORole Kind = "aws-sso-role"
	// KindAzure obtains an Entra ID token for a subscription.
	KindAzure Kind = "azure"
	// KindGCP impersonates a service account with a signed-in user.
	KindGCP Kind = "gcp"
)

// Cloud returns the cloud a kind belongs to.
func (k Kind) Cloud() Cloud {
	switch k {
	case KindAWSIAMUser, KindAWSAssumeRole, KindAWSSSORole:
		return CloudAWS
	case KindAzure:
		return CloudAzure
	case KindGCP:
		return CloudGCP
	}
	return ""
}

// Status is the lifecycle state of a session.
type Status string

const (
	StatusInactive Status = "inactive"
	StatusActive   Status = "active"
)

// Integration is an identity source that sessions are discovered from or
// authenticated through, for example an IAM Identity Center portal.
type Integration struct {
	ID    string `json:"id"`
	Alias string `json:"alias"`
	Cloud Cloud  `json:"cloud"`

	AWSSSO *AWSSSOIntegration `json:"awsSso,omitempty"`
	Azure  *AzureIntegration  `json:"azure,omitempty"`
	GCP    *GCPIntegration    `json:"gcp,omitempty"`
}

// AWSSSOIntegration configures an IAM Identity Center portal.
type AWSSSOIntegration struct {
	StartURL string `json:"startUrl"`
	Region   string `json:"region"`
	// TokenExpires is when the cached access token stops working. Nil when logged out.
	TokenExpires *time.Time `json:"tokenExpires,omitempty"`
}

// AzureIntegration configures an Entra ID tenant.
type AzureIntegration struct {
	TenantID string `json:"tenantId"`
	// Account is the signed-in user principal name. Empty when logged out.
	Account string `json:"account,omitempty"`
}

// GCPIntegration configures a Google account login.
type GCPIntegration struct {
	// Account is the signed-in Google account email. Empty when logged out.
	Account string `json:"account,omitempty"`
}

// Session is one identity or role that the user assumes.
type Session struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Kind   Kind   `json:"kind"`
	Region string `json:"region,omitempty"`
	// IntegrationID links the session to the integration that authenticates it.
	IntegrationID string `json:"integrationId,omitempty"`
	Status        Status `json:"status"`
	// Favorite pins the session to the favorites panel.
	Favorite bool `json:"favorite,omitempty"`
	// Hidden keeps the session out of the lists and the tray menu.
	Hidden bool `json:"hidden,omitempty"`
	// Tags are the names of the workspace tags this session belongs to.
	Tags []string `json:"tags,omitempty"`
	// Expires is when the current credentials stop working. Nil when inactive.
	Expires *time.Time `json:"expires,omitempty"`

	AWS   *AWSSession   `json:"aws,omitempty"`
	Azure *AzureSession `json:"azure,omitempty"`
	GCP   *GCPSession   `json:"gcp,omitempty"`
}

// AWSSession holds AWS specific settings.
type AWSSession struct {
	// Profile is the name written to ~/.aws/config. Empty means the shared "default" profile.
	Profile string `json:"profile,omitempty"`
	// AccountID and RoleName identify an SSO role or the target of AssumeRole.
	AccountID string `json:"accountId,omitempty"`
	RoleName  string `json:"roleName,omitempty"`
	// RoleARN is the role to assume. Set for KindAWSAssumeRole.
	RoleARN string `json:"roleArn,omitempty"`
	// SourceSessionID provides credentials for AssumeRole.
	SourceSessionID string `json:"sourceSessionId,omitempty"`
	// ExternalID is passed to AssumeRole when set.
	ExternalID string `json:"externalId,omitempty"`
	// MFADevice is the serial or ARN of the MFA device for IAM users.
	MFADevice string `json:"mfaDevice,omitempty"`
}

// AzureSession holds Azure specific settings.
type AzureSession struct {
	SubscriptionID string `json:"subscriptionId"`
	TenantID       string `json:"tenantId"`
}

// GCPSession holds GCP specific settings.
type GCPSession struct {
	ProjectID      string `json:"projectId"`
	ServiceAccount string `json:"serviceAccount,omitempty"`
}

// Credentials are the short-lived secrets a session yields.
type Credentials struct {
	// AWS fields.
	AccessKeyID     string `json:"accessKeyId,omitempty"`
	SecretAccessKey string `json:"secretAccessKey,omitempty"`
	SessionToken    string `json:"sessionToken,omitempty"`
	// Token is a bearer token for Azure and GCP.
	Token      string     `json:"token,omitempty"`
	Expiration *time.Time `json:"expiration,omitempty"`
}

// Expired reports whether the credentials are unusable within skew.
func (c Credentials) Expired(now time.Time, skew time.Duration) bool {
	return c.Expiration != nil && !now.Add(skew).Before(*c.Expiration)
}

// Settings are user preferences that live alongside the workspace.
type Settings struct {
	// Theme is "system", "light", or "dark".
	Theme string `json:"theme"`
	// DefaultRegion pre-fills region fields for new AWS sessions.
	DefaultRegion string `json:"defaultRegion"`
	// AssumeRoleMinutes is the requested duration for STS AssumeRole calls.
	AssumeRoleMinutes int `json:"assumeRoleMinutes"`
	// HideOnClose keeps the desktop app running in the tray when its window closes.
	HideOnClose bool `json:"hideOnClose"`
	// NotifyOff silences the desktop notifications that warn before a session
	// expires. Stored inverted so the default (zero value) keeps them on.
	NotifyOff bool `json:"notifyOff,omitempty"`
	// VerboseLogging turns on diagnostic output, the same as ROLLE_DEBUG=1.
	VerboseLogging bool `json:"verboseLogging"`
	// AutoUpdateOff disables background update checks. Stored inverted so
	// the default (zero value) keeps updates on.
	AutoUpdateOff bool `json:"autoUpdateOff"`
	// Terminal picks the terminal app for "Open terminal". Empty means auto.
	Terminal string `json:"terminal,omitempty"`
	// ProxyURL routes every request through one proxy. Empty follows the
	// HTTPS_PROXY environment.
	ProxyURL string `json:"proxyUrl,omitempty"`
	// CABundle is a PEM file of extra roots, added to the OS trust store.
	CABundle string `json:"caBundle,omitempty"`
	// UpdateChannel is "beta" to install pre-releases. Empty means stable.
	UpdateChannel string `json:"updateChannel,omitempty"`
}

// DefaultSettings are used until the user changes something.
func DefaultSettings() Settings {
	return Settings{Theme: "system", DefaultRegion: "us-east-1", AssumeRoleMinutes: 60, HideOnClose: true}
}

// Normalize fills blanks with defaults and clamps out-of-range values.
func (s Settings) Normalize() Settings {
	d := DefaultSettings()
	if s.Theme != "light" && s.Theme != "dark" {
		s.Theme = d.Theme
	}
	if s.DefaultRegion == "" {
		s.DefaultRegion = d.DefaultRegion
	}
	if s.AssumeRoleMinutes < 15 {
		s.AssumeRoleMinutes = d.AssumeRoleMinutes
	}
	if s.AssumeRoleMinutes > 12*60 {
		s.AssumeRoleMinutes = 12 * 60
	}
	s.ProxyURL = strings.TrimSpace(s.ProxyURL)
	s.CABundle = strings.TrimSpace(s.CABundle)
	if s.UpdateChannel != "beta" {
		s.UpdateChannel = ""
	}
	return s
}

// Tag is a user-defined group of sessions shown in the sidebar.
type Tag struct {
	Name string `json:"name"`
	// Color is one of TagColors. Empty means the default.
	Color string `json:"color,omitempty"`
	// Icon is one of TagIcons. Empty means the default tag icon.
	Icon string `json:"icon,omitempty"`
}

// TagColors are the colors a tag may use, by name; the interface maps them
// to its palette.
var TagColors = []string{"gray", "blue", "green", "orange", "red", "purple", "pink", "yellow"}

// TagIcons are the icons a tag may use, by name.
var TagIcons = []string{"tag", "folder", "briefcase", "shield", "flask", "rocket", "star", "building", "cloud", "wrench"}

// Workspace is everything rolle persists, except secrets.
type Workspace struct {
	Version      int           `json:"version"`
	Integrations []Integration `json:"integrations"`
	Sessions     []Session     `json:"sessions"`
	// Tags are user-defined groups, in sidebar order. Sessions refer to
	// them by name.
	Tags []Tag `json:"tags,omitempty"`
	// Onboarded is set once the desktop walkthrough completes.
	Onboarded bool `json:"onboarded"`
	// Settings holds user preferences. Nil means defaults.
	Settings *Settings `json:"settings,omitempty"`
}

// EffectiveSettings returns the stored settings with defaults applied.
func (w *Workspace) EffectiveSettings() Settings {
	if w.Settings == nil {
		return DefaultSettings()
	}
	return w.Settings.Normalize()
}

// WorkspaceVersion is the current on-disk schema version.
const WorkspaceVersion = 1

// ErrNotFound is returned when an ID does not match.
var ErrNotFound = errors.New("not found")

// Session returns the session with id.
func (w *Workspace) Session(id string) (*Session, error) {
	for i := range w.Sessions {
		if w.Sessions[i].ID == id {
			return &w.Sessions[i], nil
		}
	}
	return nil, ErrNotFound
}

// Integration returns the integration with id.
func (w *Workspace) Integration(id string) (*Integration, error) {
	for i := range w.Integrations {
		if w.Integrations[i].ID == id {
			return &w.Integrations[i], nil
		}
	}
	return nil, ErrNotFound
}

// RemoveSession deletes the session with id.
func (w *Workspace) RemoveSession(id string) error {
	for i := range w.Sessions {
		if w.Sessions[i].ID == id {
			w.Sessions = append(w.Sessions[:i], w.Sessions[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

// RemoveIntegration deletes the integration with id and every session that uses it.
func (w *Workspace) RemoveIntegration(id string) error {
	found := false
	for i := range w.Integrations {
		if w.Integrations[i].ID == id {
			w.Integrations = append(w.Integrations[:i], w.Integrations[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		return ErrNotFound
	}
	kept := w.Sessions[:0]
	for _, s := range w.Sessions {
		if s.IntegrationID != id {
			kept = append(kept, s)
		}
	}
	w.Sessions = kept
	return nil
}

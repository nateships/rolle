// Package policy reads what an organization sets for rolle with an MDM
// configuration profile, such as a Jamf profile for the com.getrolle.app
// domain. The CLI and the desktop app both read it, so both follow it.
package policy

import (
	"encoding/json"
	"net/url"
	"os"
	"slices"
	"strings"

	"howett.net/plist"

	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/debug"
)

// Domain is the preference domain of the profile.
const Domain = "com.getrolle.app"

// Policy is what the profiles set. The zero value sets nothing.
type Policy struct {
	// DisableUpdates turns the desktop updater off.
	DisableUpdates bool
	// Settings maps the JSON name of a core.Settings field to its value as
	// JSON. rolle applies each value once and the user can change it, unless
	// the key is locked.
	Settings map[string]json.RawMessage
	// Locked lists the JSON names of settings that the user cannot change.
	Locked []string
	// Integrations are the identity sources that rolle adds for the user.
	// The user can rename, remove, and sign in to them like any other.
	Integrations []Integration
}

// Integration is an identity source in a profile.
type Integration struct {
	// Type is "aws-sso" for an IAM Identity Center portal or "azure" for an
	// Entra ID tenant.
	Type  string
	Alias string
	// StartURL and Region describe an aws-sso portal. StartURL is https,
	// with a lowercase host and no trailing slash.
	StartURL string
	Region   string
	// TenantID is the lowercase tenant of an azure integration.
	TenantID string
}

// Key identifies the source: two integrations with one key are the same
// portal or tenant.
func (in Integration) Key() string {
	if in.Type == "azure" {
		return "azure:" + strings.ToLower(in.TenantID)
	}
	u, _ := startURL(in.StartURL)
	return "aws-sso:" + u
}

// Key returns the key of an integration in the workspace, or "" for a
// source that a profile cannot add.
func Key(in core.Integration) string {
	switch {
	case in.AWSSSO != nil:
		return Integration{Type: "aws-sso", StartURL: in.AWSSSO.StartURL}.Key()
	case in.Azure != nil && in.Azure.TenantID != "":
		return Integration{Type: "azure", TenantID: in.Azure.TenantID}.Key()
	}
	return ""
}

// keys maps each settings key that a profile can set to the JSON name of
// the core.Settings field. Appearance keys stay with the user. LoginItem is
// not here: it takes effect only when the app registers with the OS, and an
// MDM manages login items itself.
var keys = map[string]string{
	"DefaultRegion":     "defaultRegion",
	"AssumeRoleMinutes": "assumeRoleMinutes",
	"ProxyURL":          "proxyUrl",
	"CABundle":          "caBundle",
	"UpdateChannel":     "updateChannel",
	"AutoUpdateOff":     "autoUpdateOff",
	"HiddenSections":    "hiddenSections",
	"HideOnClose":       "hideOnClose",
	"NotifyOff":         "notifyOff",
	"NotifyLeadMinutes": "notifyLeadMinutes",
	"Terminal":          "terminal",
}

// Empty reports whether the policy sets nothing.
func (p Policy) Empty() bool {
	return !p.DisableUpdates && len(p.Settings) == 0 && len(p.Locked) == 0 && len(p.Integrations) == 0
}

// IsLocked reports whether the user cannot change the setting. A locked key
// without a value locks nothing.
func (p Policy) IsLocked(name string) bool {
	_, ok := p.Settings[name]
	return ok && slices.Contains(p.Locked, name)
}

// Load reads the profiles of this machine and of the current user.
func Load() Policy { return load(sources(), trusted) }

// profile is the shape of one profile file.
type profile struct {
	DisableUpdates *bool          `plist:"DisableUpdates"`
	Settings       map[string]any `plist:"Settings"`
	LockedSettings []string       `plist:"LockedSettings"`
	// Integrations stay untyped here, so one bad entry does not make the
	// whole file fail to decode.
	Integrations []any `plist:"Integrations"`
}

// load reads the files in order. A later file wins for each key. The
// function skips a file that is missing, that it cannot parse, or that
// trust refuses. It also skips an unknown key and a value of the wrong type.
func load(paths []string, trust func(os.FileInfo) bool) Policy {
	var p Policy
	for _, path := range paths {
		fi, err := os.Stat(path)
		if err != nil {
			continue
		}
		if !trust(fi) {
			debug.Logf("policy", "%s: not owned by root or writable by others, skipped", path)
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var pr profile
		if _, err := plist.Unmarshal(data, &pr); err != nil {
			debug.Logf("policy", "%s: %v", path, err)
			continue
		}
		if pr.DisableUpdates != nil {
			p.DisableUpdates = *pr.DisableUpdates
		}
		for key, v := range pr.Settings {
			name, raw, ok := setting(key, v)
			if !ok {
				debug.Logf("policy", "%s: setting %s skipped", path, key)
				continue
			}
			if p.Settings == nil {
				p.Settings = map[string]json.RawMessage{}
			}
			p.Settings[name] = raw
		}
		for _, key := range pr.LockedSettings {
			if name, ok := keys[key]; ok && !slices.Contains(p.Locked, name) {
				p.Locked = append(p.Locked, name)
			}
		}
		for i, v := range pr.Integrations {
			in, ok := integration(v)
			if !ok {
				debug.Logf("policy", "%s: integration %d skipped", path, i)
				continue
			}
			k := in.Key()
			if j := slices.IndexFunc(p.Integrations, func(o Integration) bool { return o.Key() == k }); j >= 0 {
				p.Integrations[j] = in
			} else {
				p.Integrations = append(p.Integrations, in)
			}
		}
	}
	return p
}

// integration reads one entry of the Integrations array. It refuses an
// entry that is not a dictionary, that has a field of the wrong type, or
// that misses a field its type needs. Google Cloud is not here: it signs in
// through gcloud, which has nothing for a profile to set.
func integration(v any) (Integration, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		return Integration{}, false
	}
	str := func(k string) (string, bool) {
		x, present := m[k]
		if !present {
			return "", true
		}
		s, ok := x.(string)
		return strings.TrimSpace(s), ok
	}
	typ, ok1 := str("Type")
	alias, ok2 := str("Alias")
	start, ok3 := str("StartURL")
	region, ok4 := str("Region")
	tenant, ok5 := str("TenantID")
	if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || alias == "" {
		return Integration{}, false
	}
	switch typ {
	case "aws-sso":
		u, ok := startURL(start)
		if !ok || region == "" {
			return Integration{}, false
		}
		return Integration{Type: typ, Alias: alias, StartURL: u, Region: strings.ToLower(region)}, true
	case "azure":
		if tenant == "" || strings.ContainsAny(tenant, " /") {
			return Integration{}, false
		}
		return Integration{Type: typ, Alias: alias, TenantID: strings.ToLower(tenant)}, true
	}
	return Integration{}, false
}

// startURL checks an Identity Center start URL and puts it in one form:
// https, a lowercase host, and no trailing slash.
func startURL(s string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return "", false
	}
	u.Host = strings.ToLower(u.Host)
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawQuery, u.Fragment = "", ""
	return u.String(), true
}

// setting maps a profile key and value to the core.Settings JSON name and
// the value as JSON. It refuses an unknown key and a value that does not
// decode into the field.
func setting(key string, v any) (string, json.RawMessage, bool) {
	name, ok := keys[key]
	if !ok {
		return "", nil, false
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return "", nil, false
	}
	probe, err := json.Marshal(map[string]json.RawMessage{name: raw})
	if err != nil {
		return "", nil, false
	}
	var s core.Settings
	if err := json.Unmarshal(probe, &s); err != nil {
		return "", nil, false
	}
	return name, raw, true
}

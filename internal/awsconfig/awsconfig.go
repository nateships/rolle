// Package awsconfig manages Rolle-owned profiles in ~/.aws/config. Each
// profile points at the Rolle CLI through credential_process, so no secret is
// written to disk.
package awsconfig

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/ini.v1"
)

// DefaultPath returns ~/.aws/config, honouring AWS_CONFIG_FILE.
func DefaultPath() (string, error) {
	if p := os.Getenv("AWS_CONFIG_FILE"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".aws", "config"), nil
}

// Profile is one Rolle-managed profile.
type Profile struct {
	Name      string
	Region    string
	SessionID string
	// Executable is the rolle binary path used in credential_process.
	Executable string
}

const (
	marker = "rolle_session"
	// preexisting records that the section existed before Rolle wrote it, so
	// Remove restores it instead of deleting it.
	preexisting = "rolle_preexisting"
	prevRegion  = "rolle_previous_region"
)

// credentialKeys make a foreign profile off limits: Rolle never overwrites
// another tool's credential configuration.
var credentialKeys = []string{"credential_process", "aws_access_key_id", "aws_secret_access_key", "aws_session_token", "sso_start_url", "sso_session", "role_arn", "source_profile", "credential_source", "web_identity_token_file", "granted_sso_start_url"}

func init() {
	// Keep "key = value" on one column so the file stays diff-friendly.
	ini.PrettyFormat = false
	ini.PrettyEqual = true
}

// CredentialsPath returns the shared credentials file that pairs with the
// config file at configPath.
func CredentialsPath(configPath string) string {
	if p := os.Getenv("AWS_SHARED_CREDENTIALS_FILE"); p != "" {
		return p
	}
	return filepath.Join(filepath.Dir(configPath), "credentials")
}

// checkShadow fails when the shared credentials file holds static keys for the
// profile. The SDK credential chain reads those before credential_process, so
// the SDK never uses the Rolle profile.
func checkShadow(configPath, profile string) error {
	credPath := CredentialsPath(configPath)
	f, err := ini.LoadSources(ini.LoadOptions{IgnoreInlineComment: true}, credPath)
	if err != nil {
		return nil // no credentials file, nothing shadows
	}
	sec, err := f.GetSection(profile)
	if err != nil {
		return nil
	}
	for _, k := range []string{"aws_access_key_id", "aws_secret_access_key", "aws_session_token"} {
		if sec.HasKey(k) {
			return fmt.Errorf("profile %q has static keys in %s that would shadow Rolle; remove that section or give the session another profile name", profile, credPath)
		}
	}
	return nil
}

// Write adds or replaces the profile in the config file at path. A profile
// that Rolle does not own is not touched.
func Write(path string, p Profile) error {
	if err := checkShadow(path, p.Name); err != nil {
		return err
	}
	f, err := load(path)
	if err != nil {
		return err
	}
	name := sectionName(p.Name)
	sec, err := f.GetSection(name)
	switch {
	case err != nil:
		if sec, err = f.NewSection(name); err != nil {
			return err
		}
	case !sec.HasKey(marker):
		// Rolle takes over a plain section (region, output, ...) and restores
		// it on Remove. A section with credentials belongs to another tool.
		for _, k := range credentialKeys {
			if sec.HasKey(k) {
				return fmt.Errorf("profile %q in %s is configured by another tool (%s)", p.Name, path, k)
			}
		}
		sec.Key(preexisting).SetValue("true")
		if sec.HasKey("region") {
			sec.Key(prevRegion).SetValue(sec.Key("region").String())
		}
	}
	if p.Region != "" {
		sec.Key("region").SetValue(p.Region)
	}
	sec.Key("credential_process").SetValue(fmt.Sprintf("%s creds --session %s", quote(p.Executable), p.SessionID))
	sec.Key(marker).SetValue(p.SessionID)
	return save(path, f)
}

// Remove deletes the profile if it belongs to sessionID. Other profiles are
// left alone.
func Remove(path, name, sessionID string) error {
	f, err := load(path)
	if err != nil {
		return err
	}
	sec, err := f.GetSection(sectionName(name))
	if err != nil || sec.Key(marker).String() != sessionID {
		return nil
	}
	if !sec.HasKey(preexisting) {
		f.DeleteSection(sec.Name())
		return save(path, f)
	}
	// Restore the section the user had before Rolle took it over.
	sec.DeleteKey("credential_process")
	sec.DeleteKey(marker)
	sec.DeleteKey(preexisting)
	if sec.HasKey(prevRegion) {
		sec.Key("region").SetValue(sec.Key(prevRegion).String())
		sec.DeleteKey(prevRegion)
	} else if sec.HasKey("region") && wroteRegion(sec) {
		sec.DeleteKey("region")
	}
	return save(path, f)
}

func sectionName(profile string) string {
	if profile == "default" {
		return "default"
	}
	return "profile " + profile
}

func quote(s string) string {
	for _, r := range s {
		if r == ' ' {
			return `"` + s + `"`
		}
	}
	return s
}

func load(path string) (*ini.File, error) {
	// The AWS CLI does not treat # or ; inside a value as a comment, so URLs
	// such as https://acme.awsapps.com/start/#/ must survive a round trip. It
	// also keeps a trailing backslash and surrounding quotes as part of the
	// value, so a foreign profile must round-trip with them intact.
	opts := ini.LoadOptions{
		AllowNestedValues:       true,
		SkipUnrecognizableLines: true,
		IgnoreInlineComment:     true,
		IgnoreContinuation:      true,
		PreserveSurroundedQuote: true,
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return ini.LoadSources(opts, []byte{})
	}
	return ini.LoadSources(opts, path)
}

func save(path string, f *ini.File) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".rolle-tmp"
	if err := f.SaveTo(tmp); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// wroteRegion reports whether Rolle added the region key: a taken-over
// section without a saved previous region had none before.
func wroteRegion(sec *ini.Section) bool { return !sec.HasKey(prevRegion) }

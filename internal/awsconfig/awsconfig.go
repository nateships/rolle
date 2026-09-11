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

const marker = "rolle_session"

func init() {
	// Keep "key = value" on one column so the file stays diff-friendly.
	ini.PrettyFormat = false
	ini.PrettyEqual = true
}

// Write adds or replaces the profile in the config file at path. A profile
// that Rolle does not own is not touched.
func Write(path string, p Profile) error {
	f, err := load(path)
	if err != nil {
		return err
	}
	name := sectionName(p.Name)
	if sec, err := f.GetSection(name); err == nil && !sec.HasKey(marker) {
		return fmt.Errorf("profile %q already exists in %s and is not managed by rolle", p.Name, path)
	}
	f.DeleteSection(name)
	sec, err := f.NewSection(name)
	if err != nil {
		return err
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
	f.DeleteSection(sec.Name())
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
	// such as https://acme.awsapps.com/start/#/ must survive a round trip.
	opts := ini.LoadOptions{AllowNestedValues: true, SkipUnrecognizableLines: true, IgnoreInlineComment: true}
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

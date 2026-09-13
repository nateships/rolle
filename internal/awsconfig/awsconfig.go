// Package awsconfig manages rolle-owned profiles in ~/.aws/config. Each
// profile points at the rolle CLI through credential_process, so no secret is
// written to disk.
package awsconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

// Profile is one rolle-managed profile.
type Profile struct {
	Name      string
	Region    string
	SessionID string
	// Executable is the rolle binary path used in credential_process.
	Executable string
}

const (
	marker = "rolle_session"
	// preexisting records that the section existed before rolle wrote it, so
	// Remove restores it instead of deleting it.
	preexisting = "rolle_preexisting"
	prevRegion  = "rolle_previous_region"
)

// credentialKeys make a foreign profile off limits: rolle never overwrites
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

// staticKeys are the credential keys a shared credentials file can hold.
var staticKeys = []string{"aws_access_key_id", "aws_secret_access_key", "aws_session_token"}

// ErrShadowed matches, through errors.Is, the error Write returns when static
// keys in the shared credentials file would take precedence over the profile.
var ErrShadowed = errors.New("static keys take precedence over the profile")

// ShadowedError is that error. Its text names the file and the profile.
type ShadowedError struct {
	Profile, Path string
}

func (e *ShadowedError) Error() string {
	return fmt.Sprintf("%s has static keys for profile %q", Display(e.Path), e.Profile)
}

// Is makes errors.Is(err, ErrShadowed) true.
func (e *ShadowedError) Is(target error) bool { return target == ErrShadowed }

// Display shortens a path under the home directory to ~/..., for messages.
func Display(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || !strings.HasPrefix(path, home+string(filepath.Separator)) {
		return path
	}
	return "~" + path[len(home):]
}

// Shadow says what keeps tools from using a rolle profile of that name.
type Shadow struct {
	// Path is the file that holds the conflicting keys.
	Path string
	// Fixable is true when RemoveStaticKeys clears the conflict: static keys
	// in the shared credentials file. A profile that another tool configures
	// in the config file needs another profile name instead.
	Fixable bool
}

// Shadowed reports what shadows a rolle profile called profile: static keys
// in the shared credentials file, which the SDK credential chain reads before
// credential_process, or another tool's credential keys in the config file.
// Nil when nothing does.
func Shadowed(configPath, profile string) *Shadow {
	credPath := CredentialsPath(configPath)
	if f, err := load(credPath); err == nil {
		if sec, err := f.GetSection(profile); err == nil {
			for _, k := range staticKeys {
				if sec.HasKey(k) {
					return &Shadow{Path: credPath, Fixable: true}
				}
			}
		}
	}
	if f, err := load(configPath); err == nil {
		if sec, err := f.GetSection(sectionName(profile)); err == nil && !sec.HasKey(marker) {
			for _, k := range credentialKeys {
				if sec.HasKey(k) {
					return &Shadow{Path: configPath}
				}
			}
		}
	}
	return nil
}

// checkShadow fails when static keys in the shared credentials file would
// shadow the profile. Write checks the config file itself.
func checkShadow(configPath, profile string) error {
	if sh := Shadowed(configPath, profile); sh != nil && sh.Fixable {
		return &ShadowedError{Profile: profile, Path: sh.Path}
	}
	return nil
}

// StaticProfile is a profile of the shared credentials file that holds
// static keys.
type StaticProfile struct {
	Name string `json:"name"`
	// Keys are the static key lines, with values masked for display.
	Keys []StaticKey `json:"keys"`
}

// StaticKey is one masked key line of a profile.
type StaticKey struct {
	Name string `json:"name"`
	// Preview shows the start of an access key ID and nothing of a secret.
	Preview string `json:"preview"`
}

// StaticProfiles lists the profiles of the shared credentials file that hold
// static keys, in file order. The file's path comes second.
func StaticProfiles(configPath string) ([]StaticProfile, string) {
	credPath := CredentialsPath(configPath)
	f, err := load(credPath)
	if err != nil {
		return nil, credPath
	}
	var out []StaticProfile
	for _, sec := range f.Sections() {
		if sec.Name() == ini.DefaultSection {
			continue
		}
		var keys []StaticKey
		for _, k := range staticKeys {
			if sec.HasKey(k) {
				keys = append(keys, StaticKey{Name: k, Preview: preview(k, sec.Key(k).String())})
			}
		}
		if len(keys) > 0 {
			out = append(out, StaticProfile{Name: sec.Name(), Keys: keys})
		}
	}
	return out, credPath
}

// preview masks a key value. An access key ID keeps its first four
// characters, which name the key type; a secret or token shows nothing.
func preview(key, value string) string {
	if key == "aws_access_key_id" && len(value) > 4 {
		return value[:4] + "…"
	}
	return "…"
}

// RemoveStaticKeys deletes the static credential keys of profile from the
// shared credentials file. A section left empty goes too. Other keys and
// other sections stay.
func RemoveStaticKeys(configPath, profile string) error {
	credPath := CredentialsPath(configPath)
	f, err := load(credPath)
	if err != nil {
		return err
	}
	sec, err := f.GetSection(profile)
	if err != nil {
		return nil
	}
	for _, k := range staticKeys {
		sec.DeleteKey(k)
	}
	if len(sec.Keys()) == 0 {
		f.DeleteSection(profile)
	}
	return save(credPath, f)
}

// Write adds or replaces the profile in the config file at path. A profile
// that rolle does not own is not touched.
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
		// rolle takes over a plain section (region, output, ...) and restores
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
	// Restore the section the user had before rolle took it over.
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
	// Write onto the target of a symlinked config, so the link stays a link.
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	// CreateTemp opens the file with owner-only permissions and a unique name.
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".rolle-*")
	if err != nil {
		return err
	}
	_, err = f.WriteTo(tmp)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// wroteRegion reports whether rolle added the region key: a taken-over
// section without a saved previous region had none before.
func wroteRegion(sec *ini.Section) bool { return !sec.HasKey(prevRegion) }

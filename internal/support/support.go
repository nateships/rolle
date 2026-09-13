// Package support builds the zip a user attaches to a bug report: version and
// platform, settings, a redacted workspace, a redacted AWS config, and the
// recent diagnostic lines. It never reads the keychain or the credential cache.
package support

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/gcp"
)

// Inputs is everything the bundle needs from the caller.
type Inputs struct {
	Version       string
	App           string // "desktop" or "cli"
	Workspace     *core.Workspace
	Settings      core.Settings
	WorkspacePath string
	CacheDir      string
	AWSConfigPath string
	Log           []string
}

var (
	reAccount  = regexp.MustCompile(`\b\d{12}\b`)
	reEmail    = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	reGUID     = regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`)
	reAWSApps  = regexp.MustCompile(`https://[A-Za-z0-9.-]+\.awsapps\.com`)
	reKeyID    = regexp.MustCompile(`\b(AKIA|ASIA)[A-Z0-9]{16}\b`)
	reSecret   = regexp.MustCompile(`(?i)(secret|token|password)([^\n=:]*[=:]\s*)\S+`)
	reUserinfo = regexp.MustCompile(`://[^/\s@]+@`)
)

// Redact replaces the values that identify an organisation or a person:
// account ids, emails, GUIDs, Identity Center portal hosts, access key ids,
// URL user info, and anything that follows secret, token, or password.
func Redact(s string) string {
	s = reSecret.ReplaceAllString(s, "${1}${2}<redacted>")
	s = reUserinfo.ReplaceAllString(s, "://<redacted>@")
	s = reKeyID.ReplaceAllString(s, "<access-key-id>")
	s = reAccount.ReplaceAllString(s, "<account>")
	s = reEmail.ReplaceAllString(s, "<email>")
	s = reGUID.ReplaceAllString(s, "<guid>")
	s = reAWSApps.ReplaceAllString(s, "https://<org>.awsapps.com")
	return s
}

// DefaultPath is a timestamped zip in the Downloads folder, or the temp dir
// when there is no Downloads folder.
func DefaultPath(now time.Time) string {
	name := "rolle-support-" + now.Format("20060102-150405") + ".zip"
	if home, err := os.UserHomeDir(); err == nil {
		if dl := filepath.Join(home, "Downloads"); isDir(dl) {
			return filepath.Join(dl, name)
		}
	}
	return filepath.Join(os.TempDir(), name)
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// Write builds the bundle into w.
func Write(w io.Writer, in Inputs) error {
	z := zip.NewWriter(w)
	add := func(name, body string) error {
		f, err := z.Create(name)
		if err != nil {
			return err
		}
		_, err = io.WriteString(f, body)
		return err
	}
	info := map[string]any{
		"version":       in.Version,
		"app":           in.App,
		"os":            runtime.GOOS,
		"arch":          runtime.GOARCH,
		"generated":     time.Now().UTC().Format(time.RFC3339),
		"workspacePath": in.WorkspacePath,
		"cacheDir":      in.CacheDir,
		"awsConfigPath": in.AWSConfigPath,
		"settings":      in.Settings,
	}
	infoJSON, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	if err := add("info.json", Redact(string(infoJSON))); err != nil {
		return err
	}
	if in.Workspace != nil {
		wsJSON, err := json.MarshalIndent(in.Workspace, "", "  ")
		if err != nil {
			return err
		}
		if err := add("workspace.json", Redact(string(wsJSON))); err != nil {
			return err
		}
	}
	if in.AWSConfigPath != "" {
		if b, err := os.ReadFile(in.AWSConfigPath); err == nil {
			if err := add("aws-config.txt", Redact(string(b))); err != nil {
				return err
			}
		}
	}
	if err := add("clouds.txt", Redact(cloudTools())); err != nil {
		return err
	}
	if err := add("log.txt", Redact(strings.Join(in.Log, "\n"))+"\n"); err != nil {
		return err
	}
	if err := add("README.txt", fmt.Sprintf("rolle support bundle, %s %s.\n\nAttach this zip to a GitHub issue: https://github.com/nateships/rolle/issues/new/choose\n\nAccount ids, emails, GUIDs, portal hosts, and secrets are replaced with <placeholders>.\nNo keychain entries or cached credentials are included.\n", in.App, in.Version)); err != nil {
		return err
	}
	return z.Close()
}

// cloudTools describes the Azure and Google Cloud command line setups rolle
// reads: where the tools are, and their non-secret profile files. Token caches
// and application default credentials are never included.
func cloudTools() string {
	var b strings.Builder
	home, _ := os.UserHomeDir()
	gcDir, _ := gcp.ConfigDir()
	for _, tool := range []string{"az", "gcloud", "aws"} {
		if p, err := exec.LookPath(tool); err == nil {
			fmt.Fprintf(&b, "%s: %s\n", tool, p)
		} else {
			fmt.Fprintf(&b, "%s: not on PATH\n", tool)
		}
	}
	azDir := os.Getenv("AZURE_CONFIG_DIR")
	if azDir == "" {
		azDir = filepath.Join(home, ".azure")
	}
	files := []string{filepath.Join(azDir, "azureProfile.json"), filepath.Join(azDir, "config"), filepath.Join(gcDir, "active_config")}
	if name, err := os.ReadFile(filepath.Join(gcDir, "active_config")); err == nil {
		files = append(files, filepath.Join(gcDir, "configurations", "config_"+strings.TrimSpace(string(name))))
	}
	adc := filepath.Join(gcDir, "application_default_credentials.json")
	if _, err := os.Stat(adc); err == nil {
		fmt.Fprintf(&b, "\n%s: present (contents withheld)\n", adc)
	} else {
		fmt.Fprintf(&b, "\n%s: absent\n", adc)
	}
	for _, f := range files {
		body, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		fmt.Fprintf(&b, "\n--- %s ---\n%s\n", f, strings.TrimSpace(string(body)))
	}
	return b.String()
}

// WriteFile builds the bundle at path, creating parent directories.
func WriteFile(path string, in Inputs) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if err := Write(f, in); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

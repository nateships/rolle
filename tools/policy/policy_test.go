// Package policy holds tests that keep the privacy promise checkable: the
// app names no network host outside the cloud providers and GitHub, and no
// dependency arrives without a change to the list here.
package policy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// hosts lists every host suffix the shipped code may name. A new entry here
// is a review event: it says the app talks to somewhere new.
var hosts = []string{
	// AWS: Identity Center portals, OIDC, STS, and the consoles.
	"amazonaws.com",
	"amazonaws.com.cn",
	"amazonaws-us-gov.com",
	"aws.amazon.com",
	"awsapps.com",
	// Azure: Entra sign-in, ARM, and the portal.
	"microsoftonline.com",
	"azure.com",
	// Google Cloud: APIs and the console.
	"googleapis.com",
	"google.com",
	// GitHub: the release feed for updates, downloads, and the issue form.
	"github.com",
	// Our own docs site and the SVG namespace in inline markup.
	"getrolle.com",
	"w3.org",
	// The loopback sign-in callback.
	"localhost",
	"127.0.0.1",
	// Placeholder text in the proxy setting.
	"proxy.corp",
}

// goModules lists the module prefixes the Go code may depend on directly.
var goModules = []string{
	"github.com/AzureAD/microsoft-authentication-library-for-go",
	"github.com/aws/aws-sdk-go-v2",
	"github.com/google/uuid",
	"github.com/spf13/cobra",
	"github.com/wailsapp/wails/v3",
	"github.com/zalando/go-keyring",
	"golang.org/x/",
	"gopkg.in/ini.v1",
}

// frontendPackages lists the npm packages the desktop frontend may depend on.
var frontendPackages = []string{
	"@tailwindcss/vite", "@testing-library/jest-dom", "@testing-library/react", "@testing-library/user-event",
	"@types/canvas-confetti", "@types/node", "@types/react", "@types/react-dom", "@vitejs/plugin-react",
	"@vitest/coverage-v8", "@wailsio/runtime", "canvas-confetti", "class-variance-authority", "cmdk", "cn",
	"jsdom", "lucide-react", "motion", "oxfmt", "oxlint", "radix-ui", "react", "react-dom", "shadcn", "sonner",
	"tailwindcss", "tw-animate-css", "typescript", "vite", "vitest",
}

var urlRE = regexp.MustCompile(`https?://([A-Za-z0-9.-]+)`)

func root(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// shipped reports whether path is source that ends up in the app or the CLI.
func shipped(path string) bool {
	if strings.Contains(path, "node_modules") || strings.Contains(path, "/dist/") || strings.Contains(path, "/bindings/") {
		return false
	}
	base := filepath.Base(path)
	if strings.HasSuffix(base, "_test.go") || strings.Contains(base, ".test.") || strings.Contains(base, ".config.") {
		return false
	}
	switch filepath.Ext(base) {
	case ".go", ".ts", ".tsx", ".html", ".css":
		return true
	}
	return false
}

func allowedHost(h string) bool {
	h = strings.ToLower(h)
	// A placeholder like "https://x" or "https://other" is not a host.
	if !strings.Contains(h, ".") && h != "localhost" {
		return true
	}
	for _, s := range hosts {
		if h == s || strings.HasSuffix(h, "."+s) {
			return true
		}
	}
	return false
}

func TestShippedCodeNamesOnlyKnownHosts(t *testing.T) {
	base := root(t)
	var bad []string
	for _, dir := range []string{"internal", "cmd", "apps/desktop"} {
		err := filepath.WalkDir(filepath.Join(base, dir), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !shipped(path) {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, m := range urlRE.FindAllStringSubmatch(string(data), -1) {
				if !allowedHost(m[1]) {
					rel, _ := filepath.Rel(base, path)
					bad = append(bad, rel+": "+m[0])
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(bad) > 0 {
		t.Fatalf("hosts outside the allow list:\n  %s", strings.Join(bad, "\n  "))
	}
}

func TestGoDependenciesAreListed(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(root(t), "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	var bad []string
	inBlock := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "require (":
			inBlock = true
			continue
		case line == ")":
			inBlock = false
			continue
		case !inBlock || strings.HasSuffix(line, "// indirect") || line == "":
			continue
		}
		mod := strings.Fields(line)[0]
		ok := false
		for _, p := range goModules {
			if mod == p || strings.HasPrefix(mod, p) {
				ok = true
				break
			}
		}
		if !ok {
			bad = append(bad, mod)
		}
	}
	if len(bad) > 0 {
		t.Fatalf("go modules outside the allow list: %s", strings.Join(bad, ", "))
	}
}

func TestFrontendDependenciesAreListed(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(root(t), "apps/desktop/frontend/package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{}
	for _, p := range frontendPackages {
		allowed[p] = true
	}
	var bad []string
	for _, deps := range []map[string]string{pkg.Dependencies, pkg.DevDependencies} {
		for name := range deps {
			if !allowed[name] {
				bad = append(bad, name)
			}
		}
	}
	if len(bad) > 0 {
		t.Fatalf("frontend packages outside the allow list: %s", strings.Join(bad, ", "))
	}
}

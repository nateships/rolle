package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// cliLink is where the rolle command is linked so every shell finds it.
const cliLink = "/usr/local/bin/rolle"

// CLIStatus describes whether the rolle command is reachable from a shell.
type CLIStatus struct {
	// Installed is true when a shell resolves rolle.
	Installed bool `json:"installed"`
	// Path is where the command resolves when Installed.
	Path string `json:"path,omitempty"`
	// Target is the command inside the app bundle.
	Target string `json:"target,omitempty"`
	// Reason is empty when Install can run: "move" when the app runs from a
	// disk image or a temporary location, "unsupported" off macOS bundles.
	Reason string `json:"reason,omitempty"`
}

// CLIStatus reports whether the bundled rolle command is on the PATH.
func (r *RolleService) CLIStatus() CLIStatus {
	exe, _ := os.Executable()
	return cliStatus(exe)
}

// InstallCLI links the bundled command into /usr/local/bin. When that
// directory is not writable, macOS asks for an administrator password once.
func (r *RolleService) InstallCLI() (CLIStatus, error) {
	exe, _ := os.Executable()
	st := cliStatus(exe)
	switch st.Reason {
	case "move":
		return st, errors.New("move Rolle to the Applications folder first")
	case "unsupported":
		return st, errors.New("the command ships inside the macOS app")
	}
	if _, err := os.Stat(st.Target); err != nil {
		return st, fmt.Errorf("the command is missing from the app: %w", err)
	}
	if err := linkCLI(st.Target); err != nil {
		return st, err
	}
	return cliStatus(exe), nil
}

// UninstallCLI removes the link. The app keeps its own copy of the command.
func (r *RolleService) UninstallCLI() error {
	if _, err := os.Lstat(cliLink); err != nil {
		return nil
	}
	if err := os.Remove(cliLink); err == nil || !errors.Is(err, os.ErrPermission) {
		return err
	}
	return adminShell(fmt.Sprintf("rm -f %s", shellQuote(cliLink)))
}

func cliStatus(exe string) CLIStatus {
	if runtime.GOOS != "darwin" {
		return CLIStatus{Reason: "unsupported"}
	}
	target := cliTarget(exe)
	if target == "" {
		return CLIStatus{Reason: "unsupported"}
	}
	// Dev bundles carry no helper; the release task adds it.
	if _, err := os.Stat(target); err != nil {
		return CLIStatus{Reason: "unsupported"}
	}
	st := CLIStatus{Target: target}
	if needsMove(target) {
		st.Reason = "move"
	}
	if p, err := exec.LookPath("rolle"); err == nil {
		st.Installed, st.Path = true, p
	} else if _, err := os.Lstat(cliLink); err == nil {
		st.Installed, st.Path = true, cliLink
	}
	return st
}

// cliTarget maps the app executable to the command inside the same bundle.
// Empty when exe is not inside a .app bundle.
func cliTarget(exe string) string {
	const marker = ".app/Contents/MacOS/"
	i := strings.Index(exe, marker)
	if i < 0 {
		return ""
	}
	return exe[:i] + ".app/Contents/Helpers/rolle"
}

// needsMove reports whether a link into this bundle would break soon: the app
// runs from a mounted disk image, from App Translocation, or from Downloads.
func needsMove(target string) bool {
	return strings.HasPrefix(target, "/Volumes/") ||
		strings.Contains(target, "/AppTranslocation/") ||
		strings.Contains(target, "/Downloads/")
}

// linkCLI writes the symlink, first as the user, then through the macOS
// administrator prompt when the directory refuses.
func linkCLI(target string) error {
	_ = os.Remove(cliLink)
	err := os.Symlink(target, cliLink)
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrPermission) && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return adminShell(fmt.Sprintf("mkdir -p /usr/local/bin && ln -sfn %s %s", shellQuote(target), shellQuote(cliLink)))
}

// adminShell runs one shell command with administrator privileges through
// osascript, which shows the standard macOS password prompt.
func adminShell(script string) error {
	as := fmt.Sprintf(`do shell script %s with administrator privileges`, appleScriptString(script))
	out, err := exec.Command("osascript", "-e", as).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if strings.Contains(msg, "User canceled") {
			return errors.New("cancelled")
		}
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("install failed: %s", msg)
	}
	return nil
}

// shellQuote wraps s in single quotes for sh.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// appleScriptString quotes s as an AppleScript string literal.
func appleScriptString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

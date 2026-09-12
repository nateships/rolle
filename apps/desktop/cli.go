package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/nateships/rolle/internal/debug"
	"github.com/nateships/rolle/internal/version"
)

// cliLink is where the macOS app links the rolle command so every shell finds it.
const cliLink = "/usr/local/bin/rolle"

// devCLIInstalled stands in for the real install in dev builds, which carry
// no command. Install and Uninstall flip it so the interface can be exercised.
var devCLIInstalled bool

// CLIStatus describes whether the rolle command is reachable from a shell.
type CLIStatus struct {
	// Installed is true when a shell resolves rolle.
	Installed bool `json:"installed"`
	// Path is where the command resolves when Installed.
	Path string `json:"path,omitempty"`
	// Target is the command the app installs: inside the bundle on macOS, a
	// file in the user's directory on Windows and Linux.
	Target string `json:"target,omitempty"`
	// Reason is empty when Install can run. "move": the macOS app runs from a
	// disk image or a temporary location. "outdated": the installed command
	// is from another version. "unsupported": this build has no command.
	Reason string `json:"reason,omitempty"`
	// Note is a hint for the user, such as opening a new terminal.
	Note string `json:"note,omitempty"`
}

// cliEnv is everything the status logic reads from the machine, so tests can
// describe any platform.
type cliEnv struct {
	goos      string
	exe       string
	home      string
	localApp  string
	payload   []byte
	lookPath  func(file string) (string, error)
	versionOf func(path string) string
	userPath  func() string
}

func liveEnv() cliEnv {
	exe, _ := os.Executable()
	home, _ := os.UserHomeDir()
	return cliEnv{
		goos:      runtime.GOOS,
		exe:       exe,
		home:      home,
		localApp:  os.Getenv("LOCALAPPDATA"),
		payload:   embeddedCLI(),
		lookPath:  exec.LookPath,
		versionOf: commandVersion,
		userPath:  userPathList,
	}
}

// CLIStatus reports whether the bundled rolle command is on the PATH.
func (r *RolleService) CLIStatus() CLIStatus {
	if devSimulated() {
		return devCLIStatus()
	}
	return cliStatusIn(liveEnv())
}

// devSimulated is true in a dev build that carries no command.
func devSimulated() bool {
	if !devMode {
		return false
	}
	st := cliStatusIn(liveEnv())
	return st.Reason == "unsupported"
}

func devCLIStatus() CLIStatus {
	st := CLIStatus{Target: "(dev build: install is simulated)"}
	if devCLIInstalled {
		st.Installed, st.Path = true, cliLink
	}
	return st
}

// InstallCLI puts the command on the PATH. macOS links the bundle's helper
// into /usr/local/bin and asks for an administrator password once when the
// directory refuses. Windows and Linux write the embedded command to a user
// directory; Windows adds that directory to the user's PATH.
func (r *RolleService) InstallCLI() (CLIStatus, error) {
	if devSimulated() {
		devCLIInstalled = true
		return devCLIStatus(), nil
	}
	env := liveEnv()
	st := cliStatusIn(env)
	switch st.Reason {
	case "move":
		return st, errors.New("move rolle to the Applications folder first")
	case "unsupported":
		return st, errors.New("this build carries no command")
	}
	if env.goos == "darwin" {
		if _, err := os.Stat(st.Target); err != nil {
			return st, fmt.Errorf("the command is missing from the app: %w", err)
		}
		if err := linkCLI(st.Target); err != nil {
			return st, err
		}
		return cliStatusIn(env), nil
	}
	if err := writeCLI(st.Target, env.payload); err != nil {
		return st, err
	}
	if env.goos == "windows" {
		if list, changed := pathListAdd(env.userPath(), filepath.Dir(st.Target)); changed {
			if err := setUserPathList(list); err != nil {
				return st, fmt.Errorf("add %s to PATH: %w", filepath.Dir(st.Target), err)
			}
		}
	}
	return cliStatusIn(env), nil
}

// UninstallCLI removes what Install made: the link on macOS, the user copy on
// Windows and Linux. A command installed another way is left alone.
func (r *RolleService) UninstallCLI() error {
	if devSimulated() {
		devCLIInstalled = false
		return nil
	}
	env := liveEnv()
	if env.goos == "darwin" {
		if _, err := os.Lstat(cliLink); err != nil {
			return nil
		}
		if err := os.Remove(cliLink); err == nil || !errors.Is(err, os.ErrPermission) {
			return err
		}
		return adminShell(fmt.Sprintf("rm -f %s", shellQuote(cliLink)))
	}
	target := userCLIPath(env)
	if target == "" {
		return nil
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if env.goos == "windows" {
		if list, changed := pathListRemove(env.userPath(), filepath.Dir(target)); changed {
			return setUserPathList(list)
		}
	}
	return nil
}

// refreshCLI rewrites the app's own copy of the command after an update, so
// the command and the app stay the same version. It never touches a command
// installed another way.
func refreshCLI() {
	env := liveEnv()
	if env.goos == "darwin" {
		return
	}
	st := cliStatusIn(env)
	if st.Reason != "outdated" || st.Path != st.Target {
		return
	}
	if err := writeCLI(st.Target, env.payload); err != nil {
		debug.Logf("cli", "refresh %s: %v", st.Target, err)
		return
	}
	debug.Logf("cli", "refreshed %s to %s", st.Target, version.Version)
}

func cliStatusIn(env cliEnv) CLIStatus {
	switch env.goos {
	case "darwin":
		return darwinStatus(env)
	case "windows", "linux":
		return userDirStatus(env)
	}
	return CLIStatus{Reason: "unsupported"}
}

// darwinStatus reads the helper inside the bundle and the link in /usr/local/bin.
func darwinStatus(env cliEnv) CLIStatus {
	target := cliTarget(env.exe)
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
	if p, err := env.lookPath("rolle"); err == nil {
		st.Installed, st.Path = true, p
	} else if _, err := os.Lstat(cliLink); err == nil {
		st.Installed, st.Path = true, cliLink
	}
	return st
}

// userDirStatus covers Windows and Linux, where the app carries the command
// and installs it into a user directory.
func userDirStatus(env cliEnv) CLIStatus {
	target := userCLIPath(env)
	if env.payload == nil || target == "" {
		return CLIStatus{Reason: "unsupported"}
	}
	st := CLIStatus{Target: target}
	if p, err := env.lookPath(cliFileName(env.goos)); err == nil {
		st.Installed, st.Path = true, p
	} else if _, err := os.Stat(target); err == nil {
		// The file is there but this process's PATH does not see it: a fresh
		// Windows install, or a Linux PATH without ~/.local/bin.
		if env.goos == "windows" && pathListHas(env.userPath(), filepath.Dir(target)) {
			st.Installed, st.Path = true, target
			st.Note = "Open a new terminal to use it."
		} else if env.goos == "linux" {
			st.Note = "Add ~/.local/bin to your PATH."
		}
	}
	if st.Installed && version.Version != "0.0.1-dev" {
		if v := env.versionOf(st.Path); v != "" && v != version.Version {
			st.Reason = "outdated"
		}
	}
	return st
}

// userCLIPath is where Windows and Linux installs write the command.
func userCLIPath(env cliEnv) string {
	switch env.goos {
	case "windows":
		if env.localApp == "" {
			return ""
		}
		return filepath.Join(env.localApp, "rolle", "bin", "rolle.exe")
	case "linux":
		if env.home == "" {
			return ""
		}
		return filepath.Join(env.home, ".local", "bin", "rolle")
	}
	return ""
}

func cliFileName(goos string) string {
	if goos == "windows" {
		return "rolle.exe"
	}
	return "rolle"
}

// writeCLI puts the payload at target through a temporary file, so a reader
// never sees a half-written command.
func writeCLI(target string, payload []byte) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o755); err != nil {
		return err
	}
	return os.Rename(tmp, target)
}

// commandVersion runs `path --version` and returns the version it prints.
func commandVersion(path string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return ""
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

// pathListHas reports whether the ;-separated Windows PATH list names dir.
func pathListHas(list, dir string) bool {
	for _, p := range strings.Split(list, ";") {
		if strings.EqualFold(strings.TrimSpace(p), dir) {
			return true
		}
	}
	return false
}

// pathListAdd appends dir to the list. The second result is false when the
// list already names it.
func pathListAdd(list, dir string) (string, bool) {
	if pathListHas(list, dir) {
		return list, false
	}
	list = strings.TrimRight(list, ";")
	if list == "" {
		return dir, true
	}
	return list + ";" + dir, true
}

// pathListRemove drops dir from the list. The second result is false when
// the list does not name it.
func pathListRemove(list, dir string) (string, bool) {
	var kept []string
	changed := false
	for _, p := range strings.Split(list, ";") {
		if strings.EqualFold(strings.TrimSpace(p), dir) {
			changed = true
			continue
		}
		kept = append(kept, p)
	}
	return strings.Join(kept, ";"), changed
}

// cliTarget maps the macOS app executable to the command inside the same
// bundle. Empty when exe is not inside a .app bundle.
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

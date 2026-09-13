// Package terminal opens the user's terminal with a session's environment
// ready. A short launcher script is written with owner-only permissions and
// deletes itself as its first action, so nothing lingers on disk.
package terminal

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// App names a terminal emulator. "auto" picks the best installed one.
type App string

const (
	Auto       App = "auto"
	MacOS      App = "terminal" // Terminal.app
	ITerm      App = "iterm"
	Ghostty    App = "ghostty"
	Warp       App = "warp"
	Cmux       App = "cmux"
	PowerShell App = "powershell"
)

// Options describe what to open.
type Options struct {
	// Title is shown to the user when the shell starts.
	Title string
	// Env is exported before the shell starts. Values are quoted safely.
	Env [][2]string
	// Dir is where the launcher script is written. Must be private.
	Dir string
	// App overrides terminal detection.
	App App
}

// Open launches a terminal window running a login shell with Env set.
func Open(o Options) error {
	if err := os.MkdirAll(o.Dir, 0o700); err != nil {
		return err
	}
	switch runtime.GOOS {
	case "windows":
		return openWindows(o)
	case "darwin":
		return openDarwin(o)
	default:
		return openLinux(o)
	}
}

func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// Exports formats env as shell assignments, one per line, for a launcher
// script in a POSIX shell or in PowerShell. Empty values are skipped.
func Exports(env [][2]string, powershell bool) string { return exports(env, powershell, false) }

// EvalExports formats env for eval in the user's shell. An empty value removes
// the variable, so a stale token from an earlier session does not stay set.
func EvalExports(env [][2]string, powershell bool) string { return exports(env, powershell, true) }

func exports(env [][2]string, powershell, unset bool) string {
	var b strings.Builder
	for _, kv := range env {
		switch {
		case kv[1] != "" && powershell:
			fmt.Fprintf(&b, "$env:%s = %s\n", kv[0], psQuote(kv[1]))
		case kv[1] != "":
			fmt.Fprintf(&b, "export %s=%s\n", kv[0], shQuote(kv[1]))
		case unset && powershell:
			fmt.Fprintf(&b, "Remove-Item Env:%s -ErrorAction SilentlyContinue\n", kv[0])
		case unset:
			fmt.Fprintf(&b, "unset %s\n", kv[0])
		}
	}
	return b.String()
}

// posixScript exports the environment, removes itself, and execs a login shell.
func posixScript(o Options) string {
	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	b.WriteString("rm -f \"$0\"\n")
	b.WriteString(Exports(o.Env, false))
	fmt.Fprintf(&b, "export ROLLE_SESSION=%s\n", shQuote(o.Title))
	fmt.Fprintf(&b, "printf '\\033[1mrolle:\\033[0m %%s ready\\n' %s\n", shQuote(o.Title))
	b.WriteString("exec \"${SHELL:-/bin/sh}\" -l\n")
	return b.String()
}

// writeScript writes body to a new private file in dir. pattern names the
// file; its "*" becomes a random string so concurrent launches do not collide.
func writeScript(dir, pattern, body string) (string, error) {
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	path := f.Name()
	err = f.Chmod(0o700)
	if err == nil {
		_, err = f.WriteString(body)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func openDarwin(o Options) (err error) {
	path, err := writeScript(o.Dir, "rolle-session-*.command", posixScript(o))
	if err != nil {
		return err
	}
	defer removeOnError(path, &err)
	app := o.App
	if app == Auto || app == "" {
		app = detectDarwin()
	}
	switch app {
	case Cmux:
		return openCmux(o, path)
	case ITerm:
		return exec.Command("osascript", "-e", `tell application "iTerm" to create window with default profile command `+appleQuote("/bin/sh "+shQuote(path))).Start()
	case Ghostty:
		return exec.Command("open", "-na", "Ghostty", "--args", "-e", "/bin/sh", path).Start()
	case Warp:
		return exec.Command("open", "-a", "Warp", path).Start()
	default:
		return exec.Command("open", "-a", "Terminal", path).Start()
	}
}

// openCmux creates a cmux workspace running the launcher. cmux is driven over
// its socket, so the app is started first when it is not running.
func openCmux(o Options, script string) error {
	cli, err := exec.LookPath("cmux")
	if err != nil {
		cli = "/opt/homebrew/bin/cmux"
	}
	args := []string{"new-workspace", "--name", "rolle: " + o.Title, "--command", "/bin/sh " + shQuote(script)}
	if err := exec.Command(cli, args...).Run(); err == nil {
		return nil
	}
	if err := exec.Command("open", "-a", "cmux").Run(); err != nil {
		return fmt.Errorf("cmux: %w", err)
	}
	var last error
	for i := 0; i < 20; i++ {
		time.Sleep(500 * time.Millisecond)
		if last = exec.Command(cli, args...).Run(); last == nil {
			return nil
		}
	}
	return fmt.Errorf("cmux did not accept the workspace: %w", last)
}

func appleQuote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"` }

// detectDarwin picks the terminal named in TERM_PROGRAM, then the first
// installed app from cmux, Ghostty, iTerm, and Warp, then Terminal.app.
func detectDarwin() App {
	if tp := os.Getenv("TERM_PROGRAM"); tp != "" {
		switch {
		case strings.Contains(tp, "iTerm"):
			return ITerm
		case strings.Contains(tp, "cmux"):
			return Cmux
		case strings.Contains(tp, "ghostty"):
			return Ghostty
		case strings.Contains(tp, "Warp"):
			return Warp
		}
	}
	for _, c := range []struct {
		path string
		app  App
	}{{"/Applications/cmux.app", Cmux}, {"/Applications/Ghostty.app", Ghostty}, {"/Applications/iTerm.app", ITerm}, {"/Applications/Warp.app", Warp}} {
		if _, err := os.Stat(c.path); err == nil {
			return c.app
		}
	}
	return MacOS
}

// removeOnError deletes the launcher script when the terminal did not start,
// so a script that holds tokens never stays behind.
func removeOnError(path string, err *error) {
	if *err != nil {
		_ = os.Remove(path)
	}
}

func openLinux(o Options) (err error) {
	path, err := writeScript(o.Dir, "rolle-session-*.sh", posixScript(o))
	if err != nil {
		return err
	}
	defer removeOnError(path, &err)
	// The script is executable, so every emulator gets one argument. This
	// also fits emulators whose -e takes a single string.
	if t := os.Getenv("TERMINAL"); t != "" {
		return exec.Command(t, "-e", path).Start()
	}
	for _, t := range []string{"x-terminal-emulator", "gnome-terminal", "konsole", "xfce4-terminal", "alacritty", "kitty", "xterm"} {
		if p, err := exec.LookPath(t); err == nil {
			if t == "gnome-terminal" {
				return exec.Command(p, "--", path).Start()
			}
			return exec.Command(p, "-e", path).Start()
		}
	}
	return fmt.Errorf("no terminal emulator found; set the TERMINAL environment variable")
}

func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

func openWindows(o Options) (err error) {
	var b strings.Builder
	b.WriteString("Remove-Item -LiteralPath $PSCommandPath -Force\n")
	b.WriteString(Exports(o.Env, true))
	fmt.Fprintf(&b, "$env:ROLLE_SESSION = %s\n", psQuote(o.Title))
	fmt.Fprintf(&b, "Write-Host ('rolle: ' + %s + ' ready')\n", psQuote(o.Title))
	path, err := writeScript(o.Dir, "rolle-session-*.ps1", b.String())
	if err != nil {
		return err
	}
	defer removeOnError(path, &err)
	args := []string{"-NoExit", "-ExecutionPolicy", "Bypass", "-File", path}
	if o.App != PowerShell {
		if wt, err := exec.LookPath("wt.exe"); err == nil {
			return exec.Command(wt, append([]string{"powershell"}, args...)...).Start()
		}
	}
	return exec.Command("powershell", args...).Start()
}

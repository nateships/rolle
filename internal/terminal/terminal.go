// Package terminal opens the user's terminal with a session's environment
// ready. A short launcher script is written with owner-only permissions and
// deletes itself as its first action. The script asks rolle for the
// environment when the shell starts, so it never holds a credential.
package terminal

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
)

// App names a terminal emulator. "auto" leaves the choice to the system: on
// macOS the app that opens .command files, on Linux $TERMINAL or the first
// emulator found, on Windows Windows Terminal when installed.
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
	// Exec and Args name the command that prints the session's environment
	// as shell exports, such as the rolle executable with "env <session>".
	// The launcher evals its output. On Windows, --powershell is appended.
	Exec string
	Args []string
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

// EvalExports formats env for eval in the user's shell. An empty credential
// removes the variable, so a stale token from an earlier session does not
// stay set. Other empty values, such as a region, leave the shell's own
// value alone.
func EvalExports(env [][2]string, powershell bool) string { return exports(env, powershell, true) }

// cleared names the variables an empty value removes in EvalExports.
var cleared = map[string]bool{"AWS_SESSION_TOKEN": true}

func exports(env [][2]string, powershell, unset bool) string {
	var b strings.Builder
	for _, kv := range env {
		switch {
		case kv[1] != "" && powershell:
			fmt.Fprintf(&b, "$env:%s = %s\n", kv[0], psQuote(kv[1]))
		case kv[1] != "":
			fmt.Fprintf(&b, "export %s=%s\n", kv[0], shQuote(kv[1]))
		case unset && cleared[kv[0]] && powershell:
			fmt.Fprintf(&b, "Remove-Item Env:%s -ErrorAction SilentlyContinue\n", kv[0])
		case unset && cleared[kv[0]]:
			fmt.Fprintf(&b, "unset %s\n", kv[0])
		}
	}
	return b.String()
}

// envCommand is the shell command that prints the environment.
func envCommand(o Options, quote func(string) string, extra ...string) string {
	parts := []string{quote(o.Exec)}
	for _, a := range slices.Concat(o.Args, extra) {
		parts = append(parts, quote(a))
	}
	return strings.Join(parts, " ")
}

// posixScript removes itself, evals the environment, and execs a login shell.
func posixScript(o Options) string {
	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	b.WriteString("rm -f \"$0\"\n")
	fmt.Fprintf(&b, "eval \"$(%s)\"\n", envCommand(o, shQuote))
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
	switch o.App {
	case Cmux:
		return openCmux(o, path)
	case ITerm:
		return exec.Command("osascript", "-e", `tell application "iTerm" to create window with default profile command `+appleQuote("/bin/sh "+shQuote(path))).Start()
	case Ghostty:
		return exec.Command("open", "-na", "Ghostty", "--args", "-e", "/bin/sh", path).Start()
	case Warp:
		return exec.Command("open", "-a", "Warp", path).Start()
	case MacOS:
		return exec.Command("open", "-a", "Terminal", path).Start()
	default:
		// The launcher is a .command file. Without an app, Launch Services
		// opens it in the terminal the user made the default for that type,
		// Terminal.app unless changed. The same thing a double-click does.
		return exec.Command("open", path).Start()
	}
}

// openCmux runs the launcher in a cmux workspace named after the session.
// The name needs the cmux command on the PATH and cmux's socket, which cmux
// opens to other processes only when its Automation setting allows it. When
// either is missing, the launcher goes through the app like a double-clicked
// .command file, which also starts cmux when it is not running. The
// workspace then takes cmux's own name.
func openCmux(o Options, script string) error {
	if cli, err := exec.LookPath("cmux"); err == nil {
		args := []string{"new-workspace", "--name", "rolle: " + o.Title, "--command", "/bin/sh " + shQuote(script)}
		if err := exec.Command(cli, args...).Run(); err == nil {
			return nil
		}
	}
	return exec.Command("open", "-a", "cmux", script).Start()
}

func appleQuote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"` }

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
	// rolle prints the reason when it fails; an empty Invoke-Expression would
	// bury it under its own error.
	fmt.Fprintf(&b, "$rolleEnv = (& %s) -join \"`n\"\n", envCommand(o, psQuote, "--powershell"))
	b.WriteString("if ($rolleEnv) { Invoke-Expression $rolleEnv }\n")
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

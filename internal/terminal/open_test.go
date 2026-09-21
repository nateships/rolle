package terminal

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// fakeBin writes an executable shell script named name into a new directory
// and returns its path. The script exits at once.
func fakeBin(t *testing.T, name string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script fakes need a POSIX shell")
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// scripts lists the launcher scripts left in dir.
func scripts(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

var testOpts = Options{Title: "Acme Prod/Admin", Exec: "/opt/rolle", Args: []string{"env", "s1", "--profile"}}

func TestOpenLinuxHonoursTerminalVariable(t *testing.T) {
	term := fakeBin(t, "myterm")
	t.Setenv("TERMINAL", term)
	t.Setenv("PATH", t.TempDir())
	o := testOpts
	o.Dir = t.TempDir()
	if err := openLinux(o); err != nil {
		t.Fatal(err)
	}
	left := scripts(t, o.Dir)
	if len(left) != 1 || !strings.HasPrefix(left[0], "rolle-session-") || !strings.HasSuffix(left[0], ".sh") {
		t.Fatalf("scripts after launch = %v", left)
	}
	// The terminal deletes the script itself. The fake does not run it, so
	// its content can be checked here.
	body, err := os.ReadFile(filepath.Join(o.Dir, left[0]))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(body); got != posixScript(o) {
		t.Fatalf("script body:\n%s", got)
	}
}

func TestOpenLinuxFindsAnEmulatorOnPath(t *testing.T) {
	kitty := fakeBin(t, "kitty")
	t.Setenv("TERMINAL", "")
	t.Setenv("PATH", filepath.Dir(kitty))
	o := testOpts
	o.Dir = t.TempDir()
	if err := openLinux(o); err != nil {
		t.Fatal(err)
	}
	if len(scripts(t, o.Dir)) != 1 {
		t.Fatal("launcher script missing after a successful start")
	}
	gnome := fakeBin(t, "gnome-terminal")
	t.Setenv("PATH", filepath.Dir(gnome))
	if err := openLinux(o); err != nil {
		t.Fatal(err)
	}
}

func TestOpenLinuxRemovesScriptWhenNoTerminalStarts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH lookups differ on Windows")
	}
	o := testOpts
	o.Dir = t.TempDir()
	t.Setenv("PATH", t.TempDir())

	t.Setenv("TERMINAL", filepath.Join(t.TempDir(), "absent-terminal"))
	if err := openLinux(o); err == nil {
		t.Fatal("expected an error for a missing TERMINAL binary")
	}
	if left := scripts(t, o.Dir); len(left) != 0 {
		t.Fatalf("a script that holds session data stayed behind: %v", left)
	}

	t.Setenv("TERMINAL", "")
	err := openLinux(o)
	if err == nil || !strings.Contains(err.Error(), "no terminal emulator found") {
		t.Fatalf("err = %v", err)
	}
	if left := scripts(t, o.Dir); len(left) != 0 {
		t.Fatalf("script left behind: %v", left)
	}
}

func TestOpenDarwinRemovesScriptWhenLauncherIsMissing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH lookups differ on Windows")
	}
	// An empty PATH hides open and osascript, so no window can start and
	// every app falls into the error path.
	t.Setenv("PATH", t.TempDir())
	for _, app := range []App{Auto, MacOS, ITerm, Ghostty, Warp, Cmux} {
		t.Run(string(app), func(t *testing.T) {
			o := testOpts
			o.Dir = t.TempDir()
			o.App = app
			if err := openDarwin(o); err == nil {
				t.Fatal("expected an error without a launcher on PATH")
			}
			if left := scripts(t, o.Dir); len(left) != 0 {
				t.Fatalf("script left behind: %v", left)
			}
		})
	}
}

func TestOpenDarwinWritesCommandScriptWhenOpenSucceeds(t *testing.T) {
	open := fakeBin(t, "open")
	t.Setenv("PATH", filepath.Dir(open))
	o := testOpts
	o.Dir = t.TempDir()
	o.App = MacOS
	if err := openDarwin(o); err != nil {
		t.Fatal(err)
	}
	left := scripts(t, o.Dir)
	if len(left) != 1 || !strings.HasSuffix(left[0], ".command") {
		t.Fatalf("scripts = %v", left)
	}
	body, _ := os.ReadFile(filepath.Join(o.Dir, left[0]))
	if string(body) != posixScript(o) {
		t.Fatalf("script body:\n%s", body)
	}
}

func TestOpenWindowsScriptContent(t *testing.T) {
	ps := fakeBin(t, "powershell")
	t.Setenv("PATH", filepath.Dir(ps))
	o := Options{Title: "it's prod", Exec: "C:\\rolle\\rolle.exe", Args: []string{"env", "s1", "--profile"}, Dir: t.TempDir(), App: PowerShell}
	if err := openWindows(o); err != nil {
		t.Fatal(err)
	}
	left := scripts(t, o.Dir)
	if len(left) != 1 || !strings.HasSuffix(left[0], ".ps1") {
		t.Fatalf("scripts = %v", left)
	}
	body, _ := os.ReadFile(filepath.Join(o.Dir, left[0]))
	s := string(body)
	rm := strings.Index(s, "Remove-Item -LiteralPath $PSCommandPath -Force\n")
	export := strings.Index(s, "$rolleEnv = (& 'C:\\rolle\\rolle.exe' 'env' 's1' '--profile' '--powershell') -join \"`n\"\nif ($rolleEnv) { Invoke-Expression $rolleEnv }\n")
	session := strings.Index(s, "$env:ROLLE_SESSION = 'it''s prod'\n")
	host := strings.Index(s, "Write-Host ('rolle: ' + 'it''s prod' + ' ready')\n")
	if rm != 0 || export < 0 || session < 0 || host < 0 || export >= session || session >= host {
		t.Fatalf("script:\n%s", s)
	}

	// Without powershell the script is removed again.
	t.Setenv("PATH", t.TempDir())
	o.Dir = t.TempDir()
	if err := openWindows(o); err == nil {
		t.Fatal("expected an error without powershell")
	}
	if left := scripts(t, o.Dir); len(left) != 0 {
		t.Fatalf("script left behind: %v", left)
	}
}

func TestOpenWindowsPrefersWindowsTerminal(t *testing.T) {
	wt := fakeBin(t, "wt.exe")
	t.Setenv("PATH", filepath.Dir(wt))
	o := Options{Title: "p", Dir: t.TempDir(), App: Auto}
	if err := openWindows(o); err != nil {
		t.Fatal(err)
	}
	// PowerShell is asked for by name and is not on PATH, so the explicit
	// choice fails while wt.exe would have worked.
	o.App = PowerShell
	o.Dir = t.TempDir()
	if err := openWindows(o); err == nil {
		t.Fatal("App=powershell must not fall back to wt.exe")
	}
}

func TestOpenCreatesPrivateLaunchDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH lookups differ on Windows")
	}
	t.Setenv("PATH", t.TempDir())
	t.Setenv("TERMINAL", filepath.Join(t.TempDir(), "absent"))
	t.Setenv("TERM_PROGRAM", "Apple_Terminal")
	dir := filepath.Join(t.TempDir(), "cache", "launch")
	o := testOpts
	o.Dir = dir
	o.App = MacOS
	if err := Open(o); err == nil {
		t.Fatal("expected an error without a terminal on PATH")
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("launch dir not created: %v", err)
	}
	if !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("launch dir mode = %o, want a 700 directory", info.Mode().Perm())
	}
	if left := scripts(t, dir); len(left) != 0 {
		t.Fatalf("script left behind: %v", left)
	}
}

func TestOpenFailsWhenDirCannotBeCreated(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Open(Options{Title: "p", Dir: filepath.Join(blocker, "launch")}); err == nil {
		t.Fatal("expected an error when Dir is under a file")
	}
}

func TestRemoveOnError(t *testing.T) {
	dir := t.TempDir()
	path, err := writeScript(dir, "rolle-session-*.sh", "x")
	if err != nil {
		t.Fatal(err)
	}
	var none error
	removeOnError(path, &none)
	if _, err := os.Stat(path); err != nil {
		t.Fatal("script removed although the launch succeeded")
	}
	failed := os.ErrPermission
	removeOnError(path, &failed)
	if _, err := os.Stat(path); err == nil {
		t.Fatal("script kept although the launch failed")
	}
}

// recordingBin writes an executable named name that appends its arguments,
// one per line, to log and exits with code.
func recordingBin(t *testing.T, name, log string, code int) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script fakes need a POSIX shell")
	}
	path := filepath.Join(t.TempDir(), name)
	body := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n' \"$a\" >> " + shQuote(log) + "; done\nexit " + fmt.Sprint(code) + "\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

// TestOpenDarwinAutoUsesTheSystemHandler checks that the default leaves the
// choice to Launch Services: open gets the script and no app.
func TestOpenDarwinAutoUsesTheSystemHandler(t *testing.T) {
	log := filepath.Join(t.TempDir(), "open.log")
	open := recordingBin(t, "open", log, 0)
	t.Setenv("PATH", filepath.Dir(open))
	o := testOpts
	o.Dir = t.TempDir()
	o.App = Auto
	if err := openDarwin(o); err != nil {
		t.Fatal(err)
	}
	// open runs in the background; it is done when the log has the arguments.
	var args []string
	for i := 0; i < 50 && len(args) == 0; i++ {
		if _, err := os.Stat(log); err == nil {
			args = readLines(t, log)
		} else {
			time.Sleep(20 * time.Millisecond)
		}
	}
	left := scripts(t, o.Dir)
	if len(args) != 1 || len(left) != 1 || args[0] != filepath.Join(o.Dir, left[0]) {
		t.Fatalf("open called with %v, scripts %v", args, left)
	}
}

// TestOpenCmuxFallsBackToTheApp checks that a refused socket does not stop
// the launch: the script goes to cmux the way a double-click would.
func TestOpenCmuxFallsBackToTheApp(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "open.log")
	cmuxLog := filepath.Join(dir, "cmux.log")
	open := recordingBin(t, "open", log, 0)
	cmux := recordingBin(t, "cmux", cmuxLog, 1)
	t.Setenv("PATH", filepath.Dir(open)+string(os.PathListSeparator)+filepath.Dir(cmux))
	o := testOpts
	o.Dir = t.TempDir()
	o.App = Cmux
	if err := openDarwin(o); err != nil {
		t.Fatal(err)
	}
	cmuxArgs := readLines(t, cmuxLog)
	if len(cmuxArgs) < 5 || cmuxArgs[0] != "new-workspace" || cmuxArgs[2] != "rolle: Acme Prod/Admin" {
		t.Fatalf("cmux called with %v", cmuxArgs)
	}
	var args []string
	for i := 0; i < 50 && len(args) == 0; i++ {
		if _, err := os.Stat(log); err == nil {
			args = readLines(t, log)
		} else {
			time.Sleep(20 * time.Millisecond)
		}
	}
	left := scripts(t, o.Dir)
	if len(args) != 3 || args[0] != "-a" || args[1] != "cmux" || len(left) != 1 || args[2] != filepath.Join(o.Dir, left[0]) {
		t.Fatalf("open called with %v, scripts %v", args, left)
	}
}

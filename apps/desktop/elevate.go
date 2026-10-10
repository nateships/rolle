package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// errorCancelled is the Win32 ERROR_CANCELLED code that ShellExecute returns
// when the user declines the UAC prompt. The elevate script exits with it.
const errorCancelled = 1223

// swapKind is how the updater puts the new release in place of the target.
type swapKind int

const (
	// swapHelper lets the updater's helper replace the target.
	swapHelper swapKind = iota
	// swapContents replaces the Contents folder of a bundle that the
	// current account can write in a folder that it cannot write.
	swapContents
	// swapElevated replaces the target through the administrator prompt.
	swapElevated
)

// updateTarget is what the updater replaces: the bundle on macOS, the
// AppImage file on Linux when the app runs from one, the executable
// elsewhere. The second result tells how to replace it. The swap needs
// elevation when the current account cannot write the target. On Windows,
// the folder of the target decides. appImage is the APPIMAGE variable the
// AppImage runtime sets; the executable itself then sits on a read-only
// mount.
func updateTarget(goos, exe, appImage string) (target string, swap swapKind) {
	switch goos {
	case "darwin":
		bundle := appBundle(exe)
		switch {
		case bundle == "":
			return "", swapHelper
		case !writable(bundle):
			return bundle, swapElevated
		case !writable(filepath.Dir(bundle)):
			return bundle, swapContents
		}
		return bundle, swapHelper
	case "windows":
		if !writable(filepath.Dir(exe)) {
			return exe, swapElevated
		}
	case "linux":
		if appImage != "" {
			return appImage, swapHelper
		}
	}
	return exe, swapHelper
}

// adminShell runs one shell command as root after the macOS administrator
// prompt, which shows prompt. Tests replace it, so they never show a real
// prompt.
var adminShell = runAsAdmin

// adminStatusMark starts the last line of output from adminScript.
const adminStatusMark = "rolle-admin-status:"

// adminScript wraps script so that its last line of output gives its exit
// code and the pid of the shell. AuthorizationExecuteWithPrivileges does not
// report the exit code, and the caller must reap the shell. The subshell
// keeps an exit in script from skipping the status line. The empty echo
// starts the status line on a new line when the output of script does not
// end with a newline.
func adminScript(script string) string {
	return fmt.Sprintf("(\n%s\n) 2>&1\ns=$?\necho\necho \"%s$s $$\"", script, adminStatusMark)
}

// adminResult splits the output of adminScript into the output of the script,
// its exit code, and the pid of the shell. ok is false when the status line
// is missing, for example because the shell did not start.
func adminResult(out []byte) (output []byte, code, pid int, ok bool) {
	text := strings.TrimRight(string(out), "\n")
	i := strings.LastIndex(text, adminStatusMark)
	if i < 0 || (i > 0 && text[i-1] != '\n') {
		return out, 0, 0, false
	}
	if _, err := fmt.Sscanf(text[i+len(adminStatusMark):], "%d %d", &code, &pid); err != nil {
		return out, 0, 0, false
	}
	return []byte(text[:i]), code, pid, true
}

// installError shapes a failed privileged command into the error the
// frontend shows. A cancelled prompt becomes the bare "cancelled", which the
// frontend hides.
func installError(out []byte, err error, cancelled bool) error {
	if cancelled {
		return errors.New("cancelled")
	}
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		msg = err.Error()
	}
	return fmt.Errorf("install failed: %s", msg)
}

// windowsSwapCommand is the cmd.exe line that puts the staged executable in
// place of the running one. Windows lets a process rename a running
// executable but not overwrite or delete it. The line copies the new file
// next to the target first, so a failed copy changes nothing. It then moves
// the running executable aside and the copy into its slot. If a move fails,
// the line moves the aside back and exits 1. It also deletes the asides of
// earlier updates, which the kernel has released by now; the unelevated app
// cannot delete them in Program Files.
func windowsSwapCommand(staged, target string) string {
	aside := fmt.Sprintf("%s.old.%d", target, time.Now().UnixNano())
	return fmt.Sprintf(`/c del /q "%[1]s.old.*" 2>nul & copy /y "%[2]s" "%[1]s.new" && move /y "%[1]s" "%[3]s" && move /y "%[1]s.new" "%[1]s" || (move /y "%[3]s" "%[1]s" & exit 1)`, target, staged, aside)
}

// windowsElevateScript is a PowerShell script that runs a cmd.exe line
// through ShellExecute with the RunAs verb, which shows the UAC prompt, and
// waits for it. A declined prompt exits with errorCancelled, which does not
// depend on the display language; other launch failures exit 1 with the
// message on stderr. Otherwise the exit code is that of cmd.exe.
func windowsElevateScript(cmdLine string) string {
	return fmt.Sprintf(`$i = New-Object System.Diagnostics.ProcessStartInfo -ArgumentList 'cmd.exe', %s; $i.Verb = 'RunAs'; $i.UseShellExecute = $true; $i.WindowStyle = 'Hidden'; try { $p = [System.Diagnostics.Process]::Start($i) } catch { if ($_.Exception.InnerException.NativeErrorCode -eq %d) { exit %d }; [Console]::Error.WriteLine($_.Exception.InnerException.Message); exit 1 }; if (-not $p) { exit 1 }; $p.WaitForExit(); exit $p.ExitCode`,
		psQuote(cmdLine), errorCancelled, errorCancelled)
}

// windowsRelaunchScript starts target once the process with pid is gone.
func windowsRelaunchScript(pid int, target string) string {
	return fmt.Sprintf(`Wait-Process -Id %d -ErrorAction SilentlyContinue; Start-Process -FilePath %s`, pid, psQuote(target))
}

// psQuote wraps s in single quotes for PowerShell.
func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

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

// updateTarget is what the updater replaces: the bundle on macOS, the
// AppImage file on Linux when the app runs from one, the executable
// elsewhere. The second result is true when the current account cannot
// replace it, so the swap needs elevation. appImage is the APPIMAGE
// variable the AppImage runtime sets; the executable itself then sits on a
// read-only mount.
func updateTarget(goos, exe, appImage string) (target string, needsElevation bool) {
	switch goos {
	case "darwin":
		bundle := appBundle(exe)
		if bundle == "" {
			return "", false
		}
		return bundle, !writable(filepath.Dir(bundle)) || !writable(bundle)
	case "windows":
		return exe, !writable(filepath.Dir(exe))
	case "linux":
		if appImage != "" {
			return appImage, false
		}
	}
	return exe, false
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

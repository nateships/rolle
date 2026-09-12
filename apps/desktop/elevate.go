package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// writable reports whether this process may create and remove entries in
// dir. It tries, because ACLs on Windows and ownership on macOS both decide
// this and neither is readable from a mode bit.
func writable(dir string) bool {
	f, err := os.CreateTemp(dir, ".rolle-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	return os.Remove(name) == nil
}

// updateTarget is what the updater replaces: the bundle on macOS, the
// executable elsewhere. The second result is true when the current account
// cannot replace it, so the swap needs elevation.
func updateTarget(goos, exe string) (target string, needsElevation bool) {
	switch goos {
	case "darwin":
		bundle := appBundle(exe)
		if bundle == "" {
			return "", false
		}
		return bundle, !writable(filepath.Dir(bundle)) || !writable(bundle)
	case "windows":
		return exe, !writable(filepath.Dir(exe))
	}
	return exe, false
}

// windowsSwapCommand is the cmd.exe line that moves the running executable
// aside and copies the staged one into place. Windows lets a running
// executable be renamed but not overwritten; the aside is swept on the next
// launch, once the kernel has released it.
func windowsSwapCommand(staged, target string) string {
	aside := fmt.Sprintf("%s.old.%d", target, time.Now().UnixNano())
	return fmt.Sprintf(`/c move /y "%s" "%s" && copy /y "%s" "%s"`, target, aside, staged, target)
}

// windowsElevateScript wraps a cmd.exe line in PowerShell's Start-Process
// with the RunAs verb, which shows the UAC prompt and waits for the command.
func windowsElevateScript(cmdLine string) string {
	return fmt.Sprintf(`$p = Start-Process -FilePath cmd.exe -ArgumentList '%s' -Verb RunAs -Wait -PassThru -WindowStyle Hidden; exit $p.ExitCode`,
		strings.ReplaceAll(cmdLine, "'", "''"))
}

// windowsRelaunchScript starts target once the process with pid is gone.
func windowsRelaunchScript(pid int, target string) string {
	return fmt.Sprintf(`Wait-Process -Id %d -ErrorAction SilentlyContinue; Start-Process -FilePath '%s'`, pid, strings.ReplaceAll(target, "'", "''"))
}

// sweepAsides removes the renamed-aside executables of earlier updates.
func sweepAsides(exe string) {
	matches, _ := filepath.Glob(exe + ".old.*")
	for _, m := range matches {
		_ = os.Remove(m)
	}
}

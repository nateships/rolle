//go:build windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// hiddenPowerShell runs script in a PowerShell with no visible window.
func hiddenPowerShell(script string) *exec.Cmd {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd
}

// elevatedSwap replaces the executable through the UAC prompt.
func elevatedSwap(staged, target string) error {
	out, err := hiddenPowerShell(windowsElevateScript(windowsSwapCommand(staged, target))).CombinedOutput()
	if err == nil {
		return nil
	}
	var exit *exec.ExitError
	return installError(out, err, errors.As(err, &exit) && exit.ExitCode() == errorCancelled)
}

// relaunchAfterExit starts the new executable once this process is gone.
func relaunchAfterExit(target string) error {
	return hiddenPowerShell(windowsRelaunchScript(os.Getpid(), target)).Start()
}

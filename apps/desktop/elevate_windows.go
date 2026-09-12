//go:build windows

package main

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

// elevatedSwap replaces the executable through the UAC prompt.
func elevatedSwap(staged, target string) error {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", windowsElevateScript(windowsSwapCommand(staged, target)))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if strings.Contains(msg, "canceled by the user") {
			return errors.New("cancelled")
		}
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("install failed: %s", msg)
	}
	return nil
}

// relaunchAfterExit starts the new executable once this process is gone.
func relaunchAfterExit(pid int, target string) error {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", windowsRelaunchScript(pid, target))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}

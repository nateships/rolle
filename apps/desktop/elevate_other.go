//go:build !darwin && !windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

func elevatedSwap(string, string) error { return errors.New("elevated install is not available here") }

// relaunchAfterExit starts target once this process is gone. The child
// outlives its parent.
func relaunchAfterExit(target string) error {
	return exec.Command("/bin/sh", "-c", waitThenRun(os.Getpid(), target)).Start()
}

// waitThenRun is a sh line that waits for pid to exit, then runs target.
func waitThenRun(pid int, target string) string {
	return fmt.Sprintf("while kill -0 %d 2>/dev/null; do sleep 0.2; done; exec %s", pid, shellQuote(target))
}

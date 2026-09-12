//go:build darwin

package main

import (
	"fmt"
	"os/exec"
)

// elevatedSwap replaces the bundle through the administrator prompt.
func elevatedSwap(staged, target string) error {
	return adminShell(fmt.Sprintf("rm -rf %s && ditto %s %s", shellQuote(target), shellQuote(staged), shellQuote(target)))
}

// relaunchAfterExit opens the bundle once this process is gone. The child
// outlives its parent.
func relaunchAfterExit(target string) error {
	return exec.Command("/bin/sh", "-c", "sleep 1; open "+shellQuote(target)).Start()
}

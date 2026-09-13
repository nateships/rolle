//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
)

// elevatedSwap replaces the bundle through the administrator prompt.
func elevatedSwap(staged, target string) error {
	return adminShell(darwinSwapCommand(staged, target))
}

// darwinSwapCommand is the sh line that puts the staged bundle in place of
// the installed one. It copies next to the target first, so a failed copy
// changes nothing, then moves the old bundle aside and the copy into its
// slot. If a move fails, the line moves the old bundle back and exits 1.
// The aside goes only once the new bundle is in place.
func darwinSwapCommand(staged, target string) string {
	t, s := shellQuote(target), shellQuote(staged)
	n, o := shellQuote(target+".new"), shellQuote(target+".old")
	return fmt.Sprintf("rm -rf %[3]s && ditto %[2]s %[3]s && mv %[1]s %[4]s && mv %[3]s %[1]s || { mv %[4]s %[1]s 2>/dev/null; rm -rf %[3]s; exit 1; }; rm -rf %[4]s", t, s, n, o)
}

// relaunchAfterExit opens the bundle once this process is gone. The child
// outlives its parent.
func relaunchAfterExit(target string) error {
	return exec.Command("/bin/sh", "-c", darwinRelaunchCommand(os.Getpid(), target)).Start()
}

// darwinRelaunchCommand waits for pid to exit, then opens target. An open
// while the old instance still runs would only bring that instance forward.
func darwinRelaunchCommand(pid int, target string) string {
	return fmt.Sprintf("while kill -0 %d 2>/dev/null; do sleep 0.2; done; open %s", pid, shellQuote(target))
}

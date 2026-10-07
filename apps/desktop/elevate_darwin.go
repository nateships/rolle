//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// elevatedSwap replaces the bundle through the administrator prompt.
func elevatedSwap(staged, target string) error {
	return adminShell(darwinSwapCommand(staged, target))
}

// darwinRequirement is the code requirement that a staged bundle must meet
// before root installs it: a Developer ID signature from the rolle team on
// the rolle bundle identifier.
const darwinRequirement = `anchor apple generic and identifier "com.getrolle.app" and certificate leaf[subject.OU] = "AMR56F4NQB"`

// darwinSwapCommand is the sh line that puts the staged bundle in place of
// the installed one. It runs as root. The staging folder belongs to the
// user, so the line first copies the bundle into a new root-only folder
// next to the target. It then makes root the owner, removes group and other
// write access, and checks the signature on that copy. Thus a change to the
// staged bundle after the check has no effect. A failed step changes
// nothing. Then the line moves the old bundle aside and the copy into its
// slot. If a move fails, the line moves the old bundle back and exits 1.
func darwinSwapCommand(staged, target string) string {
	t, s := shellQuote(target), shellQuote(staged)
	tmpl := shellQuote(filepath.Join(filepath.Dir(target), ".rolle-update.XXXXXX"))
	req := shellQuote("=" + darwinRequirement)
	return fmt.Sprintf(`d=$(mktemp -d %[3]s) || exit 1; ditto %[2]s "$d/new.app" && chown -R root:wheel "$d/new.app" && chmod -R go-w "$d/new.app" && codesign --verify --deep --strict -R %[4]s "$d/new.app" && mv %[1]s "$d/old.app" && mv "$d/new.app" %[1]s || { mv "$d/old.app" %[1]s 2>/dev/null; rm -rf "$d"; exit 1; }; rm -rf "$d"`, t, s, tmpl, req)
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

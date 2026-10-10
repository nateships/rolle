package main

import (
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

func TestCLITarget(t *testing.T) {
	got := cliTarget("/Applications/rolle.app/Contents/MacOS/rolle")
	if want := "/Applications/rolle.app/Contents/Helpers/rolle"; got != want {
		t.Fatalf("cliTarget = %q, want %q", got, want)
	}
	if got := cliTarget("/Users/nate/go/bin/rolle-desktop"); got != "" {
		t.Fatalf("cliTarget outside a bundle = %q, want empty", got)
	}
}

func TestNeedsMove(t *testing.T) {
	for path, want := range map[string]bool{
		"/Applications/rolle.app/Contents/Helpers/rolle":                               false,
		"/Users/nate/Applications/rolle.app/Contents/Helpers/rolle":                    false,
		"/Volumes/rolle/rolle.app/Contents/Helpers/rolle":                              true,
		"/Users/nate/Downloads/rolle.app/Contents/Helpers/rolle":                       true,
		"/private/var/folders/x/AppTranslocation/y/d/rolle.app/Contents/Helpers/rolle": true,
	} {
		if got := needsMove(path); got != want {
			t.Errorf("needsMove(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestShellQuoting(t *testing.T) {
	if got := shellQuote("/Apps/It's Here/rolle"); got != `'/Apps/It'\''s Here/rolle'` {
		t.Fatalf("shellQuote = %s", got)
	}
}

// adminScript runs here without root, through sh as the prompt would run it.
// The status line must survive an exit in the script and keep the output of
// the script apart.
func TestAdminScriptReportsTheExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs sh")
	}
	for _, tc := range []struct {
		script, output string
		code           int
	}{
		{"echo done", "done", 0},
		{"printf no-newline", "no-newline", 0},
		{"echo nope >&2; exit 3", "nope", 3},
		{"false || { echo rolled back; exit 1; }; echo unreachable", "rolled back", 1},
	} {
		cmd := exec.Command("/bin/sh", "-c", adminScript(tc.script))
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%q: %v", tc.script, err)
		}
		output, code, pid, ok := adminResult(out)
		if !ok || code != tc.code || strings.TrimSpace(string(output)) != tc.output || pid != cmd.Process.Pid {
			t.Fatalf("%q: output %q, code %d, pid %d (want %d), ok %v", tc.script, output, code, pid, cmd.Process.Pid, ok)
		}
	}
}

func TestAdminResultWithoutStatusLine(t *testing.T) {
	for _, out := range []string{"", "partial output\n", "text " + adminStatusMark + "0 1\n", adminStatusMark + "x\n"} {
		if _, _, _, ok := adminResult([]byte(out)); ok {
			t.Errorf("adminResult(%q) is ok", out)
		}
	}
}

package terminal

import (
	"strings"
	"testing"
)

func TestPosixScriptQuotesAndSelfDeletes(t *testing.T) {
	s := posixScript(Options{Title: "Acme Prod/Admin", Env: [][2]string{{"AWS_PROFILE", "Acme-Prod-Admin"}, {"EMPTY", ""}, {"TOKEN", "it's;rm -rf"}}})
	for _, want := range []string{"#!/bin/sh\n", "rm -f \"$0\"\n", "export AWS_PROFILE='Acme-Prod-Admin'\n", `export TOKEN='it'\''s;rm -rf'`, "printf '\\033[1mrolle:\\033[0m %s ready\\n' 'Acme Prod/Admin'\n", "exec \"${SHELL:-/bin/sh}\" -l\n"} {
		if !strings.Contains(s, want) {
			t.Fatalf("script missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "EMPTY") {
		t.Fatal("empty values should be skipped")
	}
}

func TestQuoting(t *testing.T) {
	if psQuote("a'b") != "'a''b'" || appleQuote(`x"y`) != `"x\"y"` {
		t.Fatal("quoting wrong")
	}
	if got := Exports([][2]string{{"A", "x'y"}, {"B", ""}}, true); got != "$env:A = 'x''y'\n" {
		t.Fatalf("powershell exports = %q", got)
	}
}

func TestEvalExportsUnsetsEmptyValues(t *testing.T) {
	// An empty region is left to the shell; only the token is cleared.
	env := [][2]string{{"AWS_ACCESS_KEY_ID", "A"}, {"AWS_SESSION_TOKEN", ""}, {"AWS_REGION", ""}}
	if got := EvalExports(env, false); got != "export AWS_ACCESS_KEY_ID='A'\nunset AWS_SESSION_TOKEN\n" {
		t.Fatalf("posix eval exports = %q", got)
	}
	if got := EvalExports(env, true); got != "$env:AWS_ACCESS_KEY_ID = 'A'\nRemove-Item Env:AWS_SESSION_TOKEN -ErrorAction SilentlyContinue\n" {
		t.Fatalf("powershell eval exports = %q", got)
	}
}

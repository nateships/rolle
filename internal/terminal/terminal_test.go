package terminal

import (
	"strings"
	"testing"
)

func TestPosixScriptQuotesAndSelfDeletes(t *testing.T) {
	s := posixScript(Options{Title: "Acme Prod/Admin", Env: [][2]string{{"AWS_PROFILE", "Acme-Prod-Admin"}, {"EMPTY", ""}, {"TOKEN", "it's;rm -rf"}}})
	for _, want := range []string{"#!/bin/sh\n", "rm -f \"$0\"\n", "export AWS_PROFILE='Acme-Prod-Admin'\n", `export TOKEN='it'\''s;rm -rf'`, "printf '\\033[1mRolle:\\033[0m %s ready\\n' 'Acme Prod/Admin'\n", "exec \"${SHELL:-/bin/sh}\" -l\n"} {
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

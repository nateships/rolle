package terminal

import (
	"strings"
	"testing"
)

func TestPosixScriptQuotesAndSelfDeletes(t *testing.T) {
	s := posixScript(Options{Title: "Acme Prod/Admin", Env: [][2]string{{"AWS_PROFILE", "Acme-Prod-Admin"}, {"EMPTY", ""}, {"TOKEN", "it's;rm -rf"}}})
	for _, want := range []string{"#!/bin/sh\n", "rm -f \"$0\"\n", "export AWS_PROFILE='Acme-Prod-Admin'\n", `export TOKEN='it'\''s;rm -rf'`, "exec \"${SHELL:-/bin/sh}\" -l\n"} {
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
}

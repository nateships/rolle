package terminal

import (
	"strings"
	"testing"
)

func TestPosixScriptQuotesAndSelfDeletes(t *testing.T) {
	// The session id is quoted like any argument, so a hostile name cannot
	// break out of the eval.
	s := posixScript(Options{Title: "Acme Prod/Admin", Exec: "/Applications/rolle.app/Contents/MacOS/rolle", Args: []string{"env", "it's;rm -rf", "--profile"}})
	for _, want := range []string{"#!/bin/sh\n", "rm -f \"$0\"\n", `eval "$('/Applications/rolle.app/Contents/MacOS/rolle' 'env' 'it'\''s;rm -rf' '--profile')"` + "\n", "printf '\\033[1mrolle:\\033[0m %s ready\\n' 'Acme Prod/Admin'\n", "exec \"${SHELL:-/bin/sh}\" -l\n"} {
		if !strings.Contains(s, want) {
			t.Fatalf("script missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "export AWS") || strings.Contains(s, "TOKEN") {
		t.Fatal("the launcher must not carry credentials")
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

func TestEvalExportsClearsOtherAWSSession(t *testing.T) {
	// A profile session clears the keys of an earlier session. A key session
	// clears the profile of an earlier session.
	env := [][2]string{{"AWS_ACCESS_KEY_ID", ""}, {"AWS_SECRET_ACCESS_KEY", ""}, {"AWS_PROFILE", ""}}
	if got := EvalExports(env, false); got != "unset AWS_ACCESS_KEY_ID\nunset AWS_SECRET_ACCESS_KEY\nunset AWS_PROFILE\n" {
		t.Fatalf("posix eval exports = %q", got)
	}
	if got := EvalExports(env, true); got != "Remove-Item Env:AWS_ACCESS_KEY_ID -ErrorAction SilentlyContinue\nRemove-Item Env:AWS_SECRET_ACCESS_KEY -ErrorAction SilentlyContinue\nRemove-Item Env:AWS_PROFILE -ErrorAction SilentlyContinue\n" {
		t.Fatalf("powershell eval exports = %q", got)
	}
	if got := Exports(env, false); got != "" {
		t.Fatalf("exports must skip empty values: %q", got)
	}
}

func TestEvalExportsClearsOtherGCPSession(t *testing.T) {
	// A session without a service account clears the impersonation of an
	// earlier session.
	env := [][2]string{{"CLOUDSDK_AUTH_IMPERSONATE_SERVICE_ACCOUNT", ""}, {"GOOGLE_APPLICATION_CREDENTIALS", ""}}
	if got := EvalExports(env, false); got != "unset CLOUDSDK_AUTH_IMPERSONATE_SERVICE_ACCOUNT\nunset GOOGLE_APPLICATION_CREDENTIALS\n" {
		t.Fatalf("posix eval exports = %q", got)
	}
	if got := EvalExports(env, true); got != "Remove-Item Env:CLOUDSDK_AUTH_IMPERSONATE_SERVICE_ACCOUNT -ErrorAction SilentlyContinue\nRemove-Item Env:GOOGLE_APPLICATION_CREDENTIALS -ErrorAction SilentlyContinue\n" {
		t.Fatalf("powershell eval exports = %q", got)
	}
	if got := Exports(env, false); got != "" {
		t.Fatalf("exports must skip empty values: %q", got)
	}
}

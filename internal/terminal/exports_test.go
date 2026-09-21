package terminal

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestExportsPOSIX(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string
	}{
		{"plain", "prod", "export K='prod'\n"},
		{"spaces", "a b", "export K='a b'\n"},
		{"double quotes", `say "hi"`, "export K='say \"hi\"'\n"},
		{"dollar and backtick", "$HOME `id`", "export K='$HOME `id`'\n"},
		{"single quote", "it's", `export K='it'\''s'` + "\n"},
		{"two single quotes", "a'b'c", `export K='a'\''b'\''c'` + "\n"},
		{"backslash", `a\b`, `export K='a\b'` + "\n"},
		{"newline", "a\nb", "export K='a\nb'\n"},
		{"semicolon", "x; rm -rf /", "export K='x; rm -rf /'\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Exports([][2]string{{"K", tc.value}}, false); got != tc.want {
				t.Fatalf("Exports = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExportsPowerShell(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string
	}{
		{"plain", "prod", "$env:K = 'prod'\n"},
		{"spaces", "a b", "$env:K = 'a b'\n"},
		{"dollar", "$env:X", "$env:K = '$env:X'\n"},
		{"double quotes", `"q"`, "$env:K = '\"q\"'\n"},
		{"single quote", "it's", "$env:K = 'it''s'\n"},
		{"backtick", "a`nb", "$env:K = 'a`nb'\n"},
		{"semicolon", "x; Remove-Item", "$env:K = 'x; Remove-Item'\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Exports([][2]string{{"K", tc.value}}, true); got != tc.want {
				t.Fatalf("Exports = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExportsKeepsOrderAndSkipsEmpty(t *testing.T) {
	env := [][2]string{{"B", "2"}, {"EMPTY", ""}, {"A", "1"}}
	if got := Exports(env, false); got != "export B='2'\nexport A='1'\n" {
		t.Fatalf("posix = %q", got)
	}
	if got := Exports(env, true); got != "$env:B = '2'\n$env:A = '1'\n" {
		t.Fatalf("powershell = %q", got)
	}
	if got := Exports(nil, false); got != "" {
		t.Fatalf("nil env = %q", got)
	}
}

func TestShQuote(t *testing.T) {
	cases := map[string]string{"": "''", "a": "'a'", "it's": `'it'\''s'`, "'": `''\'''`}
	for in, want := range cases {
		if got := shQuote(in); got != want {
			t.Errorf("shQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPosixScriptOrderAndTitleQuoting(t *testing.T) {
	s := posixScript(Options{Title: "it's prod", Env: [][2]string{{"AWS_PROFILE", "p"}}})
	rm := strings.Index(s, "rm -f \"$0\"\n")
	export := strings.Index(s, "export AWS_PROFILE='p'\n")
	session := strings.Index(s, `export ROLLE_SESSION='it'\''s prod'`+"\n")
	exec := strings.Index(s, "exec \"${SHELL:-/bin/sh}\" -l\n")
	if rm < 0 || export < 0 || session < 0 || exec < 0 {
		t.Fatalf("script missing a line:\n%s", s)
	}
	if rm >= export || export >= session || session >= exec {
		t.Fatalf("lines out of order (rm=%d export=%d session=%d exec=%d):\n%s", rm, export, session, exec, s)
	}
	if !strings.HasSuffix(s, "\n") || strings.Count(s, "exec ") != 1 {
		t.Fatalf("script:\n%s", s)
	}
}

func TestWriteScript(t *testing.T) {
	dir := t.TempDir()
	first, err := writeScript(dir, "rolle-session-*.sh", "body one")
	if err != nil {
		t.Fatal(err)
	}
	second, err := writeScript(dir, "rolle-session-*.sh", "body two")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("two launches got the same script path")
	}
	for path, want := range map[string]string{first: "body one", second: "body two"} {
		if filepath.Dir(path) != dir {
			t.Fatalf("script written outside dir: %s", path)
		}
		base := filepath.Base(path)
		if !strings.HasPrefix(base, "rolle-session-") || !strings.HasSuffix(base, ".sh") {
			t.Fatalf("script name = %q", base)
		}
		data, err := os.ReadFile(path)
		if err != nil || string(data) != want {
			t.Fatalf("content = %q, %v", data, err)
		}
		if runtime.GOOS != "windows" {
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if perm := info.Mode().Perm(); perm != 0o700 {
				t.Fatalf("perm = %o, want 700", perm)
			}
		}
	}
}

func TestWriteScriptFailsWithoutDir(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent")
	if _, err := writeScript(missing, "rolle-session-*.sh", "x"); err == nil {
		t.Fatal("expected an error for a missing directory")
	}
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("writeScript created the directory")
	}
}

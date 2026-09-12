package main

import "testing"

func TestCLITarget(t *testing.T) {
	got := cliTarget("/Applications/Rolle.app/Contents/MacOS/Rolle")
	if want := "/Applications/Rolle.app/Contents/Helpers/rolle"; got != want {
		t.Fatalf("cliTarget = %q, want %q", got, want)
	}
	if got := cliTarget("/Users/nate/go/bin/rolle-desktop"); got != "" {
		t.Fatalf("cliTarget outside a bundle = %q, want empty", got)
	}
}

func TestNeedsMove(t *testing.T) {
	for path, want := range map[string]bool{
		"/Applications/Rolle.app/Contents/Helpers/rolle":                               false,
		"/Users/nate/Applications/Rolle.app/Contents/Helpers/rolle":                    false,
		"/Volumes/Rolle/Rolle.app/Contents/Helpers/rolle":                              true,
		"/Users/nate/Downloads/Rolle.app/Contents/Helpers/rolle":                       true,
		"/private/var/folders/x/AppTranslocation/y/d/Rolle.app/Contents/Helpers/rolle": true,
	} {
		if got := needsMove(path); got != want {
			t.Errorf("needsMove(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestShellAndAppleScriptQuoting(t *testing.T) {
	if got := shellQuote("/Apps/It's Here/rolle"); got != `'/Apps/It'\''s Here/rolle'` {
		t.Fatalf("shellQuote = %s", got)
	}
	if got := appleScriptString(`say "hi" \ bye`); got != `"say \"hi\" \\ bye"` {
		t.Fatalf("appleScriptString = %s", got)
	}
}

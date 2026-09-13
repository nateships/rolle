//go:build darwin

package main

import "testing"

func TestDarwinSwapAndRelaunchCommands(t *testing.T) {
	swap := darwinSwapCommand("/tmp/wails-update-1/rolle.app", "/Applications/rolle.app")
	want := "rm -rf '/Applications/rolle.app.new' && ditto '/tmp/wails-update-1/rolle.app' '/Applications/rolle.app.new' && mv '/Applications/rolle.app' '/Applications/rolle.app.old' && mv '/Applications/rolle.app.new' '/Applications/rolle.app' || { mv '/Applications/rolle.app.old' '/Applications/rolle.app' 2>/dev/null; rm -rf '/Applications/rolle.app.new'; exit 1; }; rm -rf '/Applications/rolle.app.old'"
	if swap != want {
		t.Fatalf("swap = %s\nwant   %s", swap, want)
	}
	if got := darwinRelaunchCommand(42, "/Applications/rolle.app"); got != "while kill -0 42 2>/dev/null; do sleep 0.2; done; open '/Applications/rolle.app'" {
		t.Fatalf("relaunch = %s", got)
	}
}

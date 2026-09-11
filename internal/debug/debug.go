// Package debug is the single switch for verbose diagnostics across the CLI
// and desktop app. Enable it with ROLLE_DEBUG=1 or `rolle --debug`.
package debug

import (
	"log"
	"os"
	"sync/atomic"
)

var enabled atomic.Bool

func init() {
	if v := os.Getenv("ROLLE_DEBUG"); v != "" && v != "0" && v != "false" {
		enabled.Store(true)
	}
}

// Enabled reports whether debug output is on.
func Enabled() bool { return enabled.Load() }

// Enable turns debug output on for the rest of the process.
func Enable() { enabled.Store(true) }

// Logf writes one diagnostic line tagged with a scope, only when enabled.
func Logf(scope, format string, args ...any) {
	if !enabled.Load() {
		return
	}
	log.Printf("[rolle:"+scope+"] "+format, args...)
}

// Package debug is the single switch for verbose diagnostics across the CLI
// and desktop app. Enable it with ROLLE_DEBUG=1 or `rolle --debug`.
package debug

import (
	"fmt"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

var enabled atomic.Bool

// recent keeps the last lines whether or not output is enabled, so a support
// bundle carries diagnostics from before the user turned verbose logging on.
var recent = struct {
	sync.Mutex
	lines []string
	next  int
}{lines: make([]string, 500)}

func init() {
	if v := os.Getenv("ROLLE_DEBUG"); v != "" && v != "0" && v != "false" {
		enabled.Store(true)
	}
}

// Enabled reports whether debug output is on.
func Enabled() bool { return enabled.Load() }

// Enable turns debug output on for the rest of the process.
func Enable() { enabled.Store(true) }

// Set turns debug output on or off. The ROLLE_DEBUG environment variable
// always wins when it is set, so a persisted preference cannot silence it.
func Set(on bool) {
	if v := os.Getenv("ROLLE_DEBUG"); v != "" && v != "0" && v != "false" {
		enabled.Store(true)
		return
	}
	enabled.Store(on)
}

// Logf writes one diagnostic line tagged with a scope. The line always goes to
// the recent buffer; it reaches the log only when enabled.
func Logf(scope, format string, args ...any) {
	msg := fmt.Sprintf("[rolle:"+scope+"] "+format, args...)
	recent.Lock()
	recent.lines[recent.next] = time.Now().UTC().Format("2006-01-02T15:04:05Z ") + msg
	recent.next = (recent.next + 1) % len(recent.lines)
	recent.Unlock()
	if !enabled.Load() {
		return
	}
	log.Print(msg)
}

// Recent returns the buffered lines, oldest first.
func Recent() []string {
	recent.Lock()
	defer recent.Unlock()
	out := make([]string, 0, len(recent.lines))
	for i := 0; i < len(recent.lines); i++ {
		if l := recent.lines[(recent.next+i)%len(recent.lines)]; l != "" {
			out = append(out, l)
		}
	}
	return out
}

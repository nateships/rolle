package debug

import (
	"fmt"
	"io"
	"log"
	"strings"
	"testing"
)

// resetRecent empties the ring buffer and restores it after the test.
func resetRecent(t *testing.T) {
	t.Helper()
	recent.Lock()
	oldLines, oldNext := recent.lines, recent.next
	recent.lines = make([]string, len(oldLines))
	recent.next = 0
	recent.Unlock()
	t.Cleanup(func() {
		recent.Lock()
		recent.lines, recent.next = oldLines, oldNext
		recent.Unlock()
	})
}

func TestRecentKeepsLinesWhileOutputIsOff(t *testing.T) {
	resetRecent(t)
	prev := enabled.Load()
	t.Cleanup(func() { enabled.Store(prev) })
	enabled.Store(false)
	log.SetOutput(io.Discard)
	defer log.SetOutput(nil)

	if got := Recent(); len(got) != 0 {
		t.Fatalf("empty buffer returned %v", got)
	}
	Logf("a", "first %d", 1)
	Logf("b", "second")
	got := Recent()
	if len(got) != 2 {
		t.Fatalf("Recent = %v", got)
	}
	if !strings.HasSuffix(got[0], " [rolle:a] first 1") || !strings.HasSuffix(got[1], " [rolle:b] second") {
		t.Fatalf("lines out of order or malformed: %v", got)
	}
	// Each line starts with a UTC timestamp.
	if len(got[0]) < 21 || got[0][10] != 'T' || got[0][19] != 'Z' {
		t.Fatalf("timestamp prefix missing: %q", got[0])
	}
}

func TestRecentWrapsAtFiveHundredLines(t *testing.T) {
	resetRecent(t)
	prev := enabled.Load()
	t.Cleanup(func() { enabled.Store(prev) })
	enabled.Store(false)

	for i := 0; i < 620; i++ {
		Logf("wrap", "line %03d", i)
	}
	got := Recent()
	if len(got) != 500 {
		t.Fatalf("Recent kept %d lines, want 500", len(got))
	}
	if !strings.HasSuffix(got[0], "line 120") {
		t.Fatalf("oldest line = %q, want line 120", got[0])
	}
	if !strings.HasSuffix(got[499], "line 619") {
		t.Fatalf("newest line = %q, want line 619", got[499])
	}
	for i := 1; i < len(got); i++ {
		want := fmt.Sprintf("line %03d", 120+i)
		if !strings.HasSuffix(got[i], want) {
			t.Fatalf("line %d = %q, want suffix %q", i, got[i], want)
		}
	}
}

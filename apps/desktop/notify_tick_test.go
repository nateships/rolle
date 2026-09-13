package main

import (
	"testing"

	"github.com/nateships/rolle/internal/core"
)

func TestNotifierTickWithoutServiceTracksSessions(t *testing.T) {
	n := newNotifier(nil, nil)
	if n.svc != nil || n.warned == nil {
		t.Fatalf("notifier = %+v", n)
	}
	// A missing workspace changes nothing.
	n.tick(nil, core.Settings{})
	if n.prev != nil {
		t.Fatal("nil workspace recorded")
	}
	w := &core.Workspace{Sessions: []core.Session{{ID: "a", Status: core.StatusActive}}}
	n.tick(w, core.Settings{})
	if len(n.prev) != 1 || n.prev[0].ID != "a" {
		t.Fatalf("prev = %+v", n.prev)
	}
	// Silenced notifications still track the sessions for the next tick.
	w = &core.Workspace{Sessions: []core.Session{{ID: "b"}}}
	n.tick(w, core.Settings{NotifyOff: true})
	if len(n.prev) != 1 || n.prev[0].ID != "b" {
		t.Fatalf("prev after notify off = %+v", n.prev)
	}
}

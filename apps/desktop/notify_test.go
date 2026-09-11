package main

import (
	"testing"
	"time"

	"github.com/nateships/rolle/internal/core"
)

func TestExpiryNotices(t *testing.T) {
	now := time.Now()
	at := func(d time.Duration) *time.Time { x := now.Add(d); return &x }
	warned := map[string]time.Time{}
	active := core.Session{ID: "a", Name: "Acme Prod/Admin", Status: core.StatusActive, Expires: at(90 * time.Second)}
	far := core.Session{ID: "b", Name: "far", Status: core.StatusActive, Expires: at(40 * time.Minute)}

	got := expiryNotices(nil, []core.Session{active, far}, now, warned)
	if len(got) != 1 || got[0].ID != "expiry-a" {
		t.Fatalf("first pass: %+v", got)
	}
	// The same expiry warns once.
	if got := expiryNotices([]core.Session{active, far}, []core.Session{active, far}, now.Add(30*time.Second), warned); len(got) != 0 {
		t.Fatalf("repeat warning: %+v", got)
	}
	// A renewed session warns again for its new expiry.
	renewed := active
	renewed.Expires = at(60 * time.Minute)
	if got := expiryNotices([]core.Session{active}, []core.Session{renewed}, now, warned); len(got) != 0 {
		t.Fatalf("renewed far away: %+v", got)
	}
	renewed.Expires = at(60 * time.Second)
	if got := expiryNotices([]core.Session{renewed}, []core.Session{renewed}, now, warned); len(got) != 1 {
		t.Fatalf("renewed close: %+v", got)
	}
	// Deactivation after the expiry passed announces the end and clears the warning.
	ended := active
	ended.Status = core.StatusInactive
	ended.Expires = nil
	got = expiryNotices([]core.Session{active}, []core.Session{ended}, now.Add(3*time.Minute), warned)
	if len(got) != 1 || got[0].ID != "expired-a" {
		t.Fatalf("expired: %+v", got)
	}
	if _, ok := warned["a"]; ok {
		t.Fatal("warning state kept after expiry")
	}
	// A manual stop before expiry is silent.
	stopped := far
	stopped.Status = core.StatusInactive
	if got := expiryNotices([]core.Session{far}, []core.Session{stopped}, now, warned); len(got) != 0 {
		t.Fatalf("manual stop notified: %+v", got)
	}
}

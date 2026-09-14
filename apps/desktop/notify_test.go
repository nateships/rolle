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

	got := expiryNotices(nil, []core.Session{active, far}, now, warned, core.DefaultNotifyLead)
	if len(got) != 1 || got[0].ID != "expiry-a" {
		t.Fatalf("first pass: %+v", got)
	}
	// The same expiry warns once.
	if got := expiryNotices([]core.Session{active, far}, []core.Session{active, far}, now.Add(30*time.Second), warned, core.DefaultNotifyLead); len(got) != 0 {
		t.Fatalf("repeat warning: %+v", got)
	}
	// A renewed session warns again for its new expiry.
	renewed := active
	renewed.Expires = at(60 * time.Minute)
	if got := expiryNotices([]core.Session{active}, []core.Session{renewed}, now, warned, core.DefaultNotifyLead); len(got) != 0 {
		t.Fatalf("renewed far away: %+v", got)
	}
	renewed.Expires = at(60 * time.Second)
	if got := expiryNotices([]core.Session{renewed}, []core.Session{renewed}, now, warned, core.DefaultNotifyLead); len(got) != 1 {
		t.Fatalf("renewed close: %+v", got)
	}
	// Deactivation after the expiry passed announces the end and clears the warning.
	ended := active
	ended.Status = core.StatusInactive
	ended.Expires = nil
	got = expiryNotices([]core.Session{active}, []core.Session{ended}, now.Add(3*time.Minute), warned, core.DefaultNotifyLead)
	if len(got) != 1 || got[0].ID != "expired-a" {
		t.Fatalf("expired: %+v", got)
	}
	if _, ok := warned["a"]; ok {
		t.Fatal("warning state kept after expiry")
	}
	// A manual stop before expiry is silent.
	stopped := far
	stopped.Status = core.StatusInactive
	if got := expiryNotices([]core.Session{far}, []core.Session{stopped}, now, warned, core.DefaultNotifyLead); len(got) != 0 {
		t.Fatalf("manual stop notified: %+v", got)
	}
}

func TestPortalNoticesAndExpiringSoon(t *testing.T) {
	now := time.Now()
	at := func(d time.Duration) *time.Time { x := now.Add(d); return &x }
	warned := map[string]time.Time{}
	w := &core.Workspace{
		Integrations: []core.Integration{
			// An Azure tenant has no portal token and must not be read as one.
			{ID: "az", Alias: "contoso", Azure: &core.AzureIntegration{}},
			{ID: "i1", Alias: "acme", AWSSSO: &core.AWSSSOIntegration{TokenExpires: at(10 * time.Minute)}},
			{ID: "i2", Alias: "idle", AWSSSO: &core.AWSSSOIntegration{TokenExpires: at(10 * time.Minute)}},
			{ID: "i3", Alias: "far", AWSSSO: &core.AWSSSOIntegration{TokenExpires: at(2 * time.Hour)}},
		},
		Sessions: []core.Session{
			{ID: "a", IntegrationID: "i1", Status: core.StatusActive, Expires: at(50 * time.Minute)},
			{ID: "b", IntegrationID: "i2", Status: core.StatusInactive},
			{ID: "c", IntegrationID: "i3", Status: core.StatusActive, Expires: at(50 * time.Minute)},
		},
	}
	// Only the portal with an active session and an expiry inside the lead warns.
	got := portalNotices(w, now, warned)
	if len(got) != 1 || got[0].ID != "portal-i1" || got[0].Category != categorySignIn || got[0].Data["integrationId"] != "i1" {
		t.Fatalf("portal notices: %+v", got)
	}
	if got := portalNotices(w, now.Add(time.Minute), warned); len(got) != 0 {
		t.Fatalf("repeat warning: %+v", got)
	}
	// A new sign-in warns again for its own expiry.
	w.Integrations[1].AWSSSO.TokenExpires = at(8 * time.Hour)
	if got := portalNotices(w, now, warned); len(got) != 0 {
		t.Fatalf("fresh sign-in: %+v", got)
	}
	w.Integrations[1].AWSSSO.TokenExpires = at(5 * time.Minute)
	if got := portalNotices(w, now, warned); len(got) != 1 {
		t.Fatalf("second warning: %+v", got)
	}

	// The tray flag follows the same rules, with the session lead from settings.
	if !expiringSoon(w, now, core.DefaultNotifyLead) {
		t.Fatal("portal inside its lead: not flagged")
	}
	// A login that renews itself is never the reason for the flag.
	w.Integrations[1].AWSSSO.TokenExpires = at(5 * time.Minute)
	w.Integrations[1].AWSSSO.Renews = true
	if expiringSoon(w, now, core.DefaultNotifyLead) {
		t.Fatal("renewing login flagged")
	}
	w.Integrations[1].AWSSSO.Renews = false
	w.Integrations[1].AWSSSO.TokenExpires = at(8 * time.Hour)
	if expiringSoon(w, now, core.DefaultNotifyLead) {
		t.Fatal("nothing close: flagged")
	}
	if !expiringSoon(w, now, time.Hour) {
		t.Fatal("session inside a one hour lead: not flagged")
	}
	if trayLabel(2, true) != "2!" || trayLabel(2, false) != "2" || trayLabel(0, true) != "" {
		t.Fatal("tray label")
	}
	if userAction("com.apple.UNNotificationDefaultActionIdentifier") != "" || userAction(actionStart) != actionStart {
		t.Fatal("user action mapping")
	}
}

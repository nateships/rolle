package main

import (
	"testing"
	"time"

	"github.com/nateships/rolle/internal/core"
)

func TestTooltipAndCountLabel(t *testing.T) {
	if got := tooltip(nil); got != "Rolle · no active sessions" {
		t.Fatal(got)
	}
	soon := time.Now().Add(5 * time.Minute)
	later := time.Now().Add(3 * time.Hour)
	got := tooltip([]core.Session{{Name: "a", Expires: &later}, {Name: "b", Expires: &soon}})
	if got != "Rolle · 2 active sessions · next expiry in 5m" && got != "Rolle · 2 active sessions · next expiry in 4m" {
		t.Fatal(got)
	}
	if countLabel(1) != "1 active session" {
		t.Fatal(countLabel(1))
	}
}

func TestAccountLabel(t *testing.T) {
	s := core.Session{Name: "Acme Prod/AdministratorAccess", AWS: &core.AWSSession{AccountID: "123"}}
	if accountLabel(nil, s) != "Acme Prod" {
		t.Fatal(accountLabel(nil, s))
	}
	s.Name = "loose"
	if accountLabel(nil, s) != "123" {
		t.Fatal(accountLabel(nil, s))
	}
}

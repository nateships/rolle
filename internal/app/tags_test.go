package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/nateships/rolle/internal/core"
)

func tagNames(tags []core.Tag) string {
	names := make([]string, 0, len(tags))
	for _, t := range tags {
		names = append(names, t.Name)
	}
	return strings.Join(names, ",")
}

func TestTagsLifecycle(t *testing.T) {
	s := testService(t)
	addSession(t, s, core.Session{ID: "a", Name: "prod-admin", Kind: core.KindAWSIAMUser, Status: core.StatusInactive})
	addSession(t, s, core.Session{ID: "b", Name: "personal", Kind: core.KindAWSIAMUser, Status: core.StatusInactive})

	if err := s.AddTag(core.Tag{Name: "  Prod   Accounts ", Color: "#FF0000", Icon: "shield"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddTag(core.Tag{Name: "prod accounts"}); !errors.Is(err, ErrTagExists) {
		t.Fatalf("duplicate in another case = %v", err)
	}
	if err := s.AddTag(core.Tag{Name: " "}); err == nil {
		t.Fatal("empty name accepted")
	}
	if err := s.AddTag(core.Tag{Name: strings.Repeat("x", maxTagLength+1)}); err == nil {
		t.Fatal("long name accepted")
	}
	if err := s.AddTag(core.Tag{Name: "Odd", Color: "red"}); err == nil {
		t.Fatal("color that is not #rrggbb accepted")
	}
	if err := s.AddTag(core.Tag{Name: "Odd", Icon: "Two Words"}); err == nil {
		t.Fatal("icon that is not a lucide name accepted")
	}
	if err := s.AddTag(core.Tag{Name: "Sandbox"}); err != nil {
		t.Fatal(err)
	}
	w, _ := s.Load()
	if tagNames(w.Tags) != "Prod Accounts,Sandbox" || w.Tags[0].Color != "#ff0000" || w.Tags[0].Icon != "shield" {
		t.Fatalf("tags = %+v", w.Tags)
	}

	// Tagging matches the tag case-insensitively and stores the tag's spelling.
	if err := s.SetSessionTag("prod-admin", "prod accounts", true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSessionTag("prod-admin", "Sandbox", true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSessionTag("personal", "Nope", true); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown tag = %v", err)
	}
	w, _ = s.Load()
	if got, _ := FindSession(w, "prod-admin"); strings.Join(got.Tags, ",") != "Prod Accounts,Sandbox" {
		t.Fatalf("session tags = %v", got.Tags)
	}
	saves := 0
	s.OnChange = func() { saves++ }
	if err := s.SetSessionTag("prod-admin", "Sandbox", true); err != nil || saves != 0 {
		t.Fatalf("repeat tag: err=%v saves=%d", err, saves)
	}
	s.OnChange = nil

	// Update renames on the sessions too and keeps the other fields it is given.
	if err := s.UpdateTag("prod accounts", core.Tag{Name: "Production", Color: "#00ce78", Icon: "rocket"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateTag("Sandbox", core.Tag{Name: "production"}); !errors.Is(err, ErrTagExists) {
		t.Fatalf("rename onto another tag = %v", err)
	}
	if err := s.UpdateTag("Ghost", core.Tag{Name: "x"}); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("update unknown = %v", err)
	}
	w, _ = s.Load()
	got, _ := FindSession(w, "prod-admin")
	if strings.Join(got.Tags, ",") != "Production,Sandbox" || w.Tags[0] != (core.Tag{Name: "Production", Color: "#00ce78", Icon: "rocket"}) {
		t.Fatalf("after update: tags=%+v session=%v", w.Tags, got.Tags)
	}

	// Move reorders the sidebar; out-of-range indexes clamp.
	if err := s.MoveTag("Sandbox", 99); err != nil {
		t.Fatal(err)
	}
	w, _ = s.Load()
	if tagNames(w.Tags) != "Production,Sandbox" {
		t.Fatalf("after clamped move = %v", tagNames(w.Tags))
	}
	if err := s.MoveTag("Sandbox", 0); err != nil {
		t.Fatal(err)
	}
	w, _ = s.Load()
	if tagNames(w.Tags) != "Sandbox,Production" {
		t.Fatalf("after move to front = %v", tagNames(w.Tags))
	}

	// Untag and remove.
	if err := s.SetSessionTag("prod-admin", "Sandbox", false); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveTag("Production"); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveTag("Production"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("remove twice = %v", err)
	}
	w, _ = s.Load()
	got, _ = FindSession(w, "prod-admin")
	if tagNames(w.Tags) != "Sandbox" || len(got.Tags) != 0 {
		t.Fatalf("after remove: tags=%+v session=%v", w.Tags, got.Tags)
	}
}

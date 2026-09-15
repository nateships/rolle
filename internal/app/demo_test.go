package app

import (
	"os"
	"strings"
	"testing"

	"github.com/nateships/rolle/internal/core"
)

func TestDemoSeedsAnIsolatedWorkspace(t *testing.T) {
	s, err := Demo()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(strings.TrimSuffix(s.WorkspacePath, "/workspace.json")) })
	w, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !w.Onboarded || len(w.Integrations) != 4 || len(w.Sessions) != 11 {
		t.Fatalf("integrations=%d sessions=%d onboarded=%v", len(w.Integrations), len(w.Sessions), w.Onboarded)
	}
	active := 0
	for _, sess := range w.Sessions {
		if sess.Status != core.StatusActive {
			continue
		}
		active++
		if _, err := s.Cache.Get(sess.ID); err != nil {
			t.Fatalf("%s has no cached credentials: %v", sess.Name, err)
		}
	}
	if active != 4 {
		t.Fatalf("active = %d", active)
	}
	cfg, err := os.ReadFile(s.AWSConfigPath)
	if err != nil || !strings.Contains(string(cfg), "[default]") {
		t.Fatalf("aws config: %v\n%s", err, cfg)
	}
	if strings.Contains(s.WorkspacePath, "/.config/") || strings.Contains(s.AWSConfigPath, ".aws") {
		t.Fatalf("demo touched real paths: %s %s", s.WorkspacePath, s.AWSConfigPath)
	}
}

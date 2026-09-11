package workspace

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/nateships/rolle/internal/core"
)

func TestLoadMissingReturnsEmpty(t *testing.T) {
	w, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatal(err)
	}
	if w.Version != core.WorkspaceVersion || len(w.Sessions) != 0 {
		t.Fatalf("workspace = %+v", w)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "workspace.json")
	in := &core.Workspace{
		Integrations: []Integration(nil),
		Sessions:     []core.Session{{ID: "s1", Name: "prod", Kind: core.KindAWSSSORole}},
	}
	if err := Save(path, in); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); runtime.GOOS != "windows" && perm != 0o600 {
		t.Fatalf("perm = %o, want 600", perm)
	}
	out, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Sessions) != 1 || out.Sessions[0].Name != "prod" {
		t.Fatalf("loaded = %+v", out)
	}
}

func TestLoadRejectsNewerVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.json")
	if err := os.WriteFile(path, []byte(`{"version": 99}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for newer version")
	}
}

type Integration = core.Integration

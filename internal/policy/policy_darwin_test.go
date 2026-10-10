//go:build darwin

package policy

import (
	"os"
	"path/filepath"
	"testing"
)

// A file that the user owns is not trusted, even with a valid profile in it.
func TestTrustedRefusesUserFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), Domain+".plist")
	data, err := os.ReadFile("testdata/computer.plist")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if p := load([]string{path}, trusted); !p.Empty() {
		t.Fatalf("policy = %+v", p)
	}
}

// Root owns /etc/hosts and only root can write it.
func TestTrustedAcceptsRootFile(t *testing.T) {
	fi, err := os.Stat("/etc/hosts")
	if err != nil {
		t.Skip(err)
	}
	if !trusted(fi) {
		t.Fatal("trusted(/etc/hosts) = false")
	}
}

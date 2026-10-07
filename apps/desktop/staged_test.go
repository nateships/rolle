package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stagedBundle writes a rolle.app with the shipped Info.plist, stamped with
// version the way the release workflow stamps it.
func stagedBundle(t *testing.T, version string) string {
	t.Helper()
	plist, err := os.ReadFile(filepath.Join("build", "darwin", "Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(t.TempDir(), "rolle.app")
	if err := os.MkdirAll(filepath.Join(app, "Contents"), 0o755); err != nil {
		t.Fatal(err)
	}
	stamped := strings.ReplaceAll(string(plist), "0.0.1", version)
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(stamped), 0o644); err != nil {
		t.Fatal(err)
	}
	return app
}

func TestCheckStagedBundle(t *testing.T) {
	app := stagedBundle(t, "1.2.3")
	if err := checkStaged("darwin", app, "1.2.3"); err != nil {
		t.Fatal(err)
	}
	if err := checkStaged("darwin", app, "1.2.4"); err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("other version: %v", err)
	}
	if err := checkStaged("darwin", filepath.Dir(app), "1.2.3"); err == nil {
		t.Fatal("folder without .app accepted")
	}
	if err := checkStaged("darwin", filepath.Join(t.TempDir(), "rolle.app"), "1.2.3"); err == nil {
		t.Fatal("bundle without Info.plist accepted")
	}
	other := filepath.Join(app, "Contents", "Info.plist")
	data, _ := os.ReadFile(other)
	if err := os.WriteFile(other, []byte(strings.Replace(string(data), bundleID, "com.example.app", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checkStaged("darwin", app, "1.2.3"); err == nil || !strings.Contains(err.Error(), "com.example.app") {
		t.Fatalf("other bundle id: %v", err)
	}
}

func TestPlistStringsReadsOnlyTheTopLevelDict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Info.plist")
	plist := `<?xml version="1.0"?><plist version="1.0"><dict>
		<key>Nested</key><dict><key>CFBundleShortVersionString</key><string>9.9.9</string></dict>
		<key>List</key><array><string>9.9.9</string></array>
		<key>Flag</key><true/>
		<key>CFBundleShortVersionString</key><string> 1.2.3 </string>
	</dict></plist>`
	if err := os.WriteFile(path, []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := plistStrings(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got["CFBundleShortVersionString"] != "1.2.3" {
		t.Fatalf("strings = %v", got)
	}
	if err := os.WriteFile(path, []byte("bplist00"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := plistStrings(path); err == nil {
		t.Fatal("binary plist accepted")
	}
}

func TestCheckStagedExecutables(t *testing.T) {
	dir := t.TempDir()
	pe := filepath.Join(dir, "rolle.exe")
	elf := filepath.Join(dir, "rolle.AppImage")
	if err := os.WriteFile(pe, []byte("MZ\x90\x00"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(elf, []byte("\x7fELF\x02"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		goos, path string
		ok         bool
	}{
		{"windows", pe, true},
		{"windows", elf, false},
		{"linux", elf, true},
		{"linux", pe, false},
		{"linux", dir, false},
		{"windows", filepath.Join(dir, "missing.exe"), false},
	} {
		if err := checkStaged(c.goos, c.path, "1.2.3"); (err == nil) != c.ok {
			t.Errorf("checkStaged(%s, %s) = %v", c.goos, filepath.Base(c.path), err)
		}
	}
}

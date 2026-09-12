package main

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportSupportBundleWritesZipToDownloads(t *testing.T) {
	r := testRolle(t)
	addIAMUser(t, r, "bundled")
	downloads := filepath.Join(os.Getenv("HOME"), "Downloads")
	if err := os.MkdirAll(downloads, 0o755); err != nil {
		t.Fatal(err)
	}
	// An empty PATH keeps the file manager closed: reveal finds no opener.
	t.Setenv("PATH", t.TempDir())

	path, err := r.ExportSupportBundle()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != downloads || !strings.HasSuffix(path, ".zip") {
		t.Fatalf("path = %s", path)
	}
	z, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = z.Close() }()
	if len(z.File) == 0 {
		t.Fatal("empty bundle")
	}
	// Access key ids are redacted everywhere in the bundle.
	for _, f := range z.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "AKIAEXAMPLE") {
			t.Fatalf("%s leaks the access key id", f.Name)
		}
	}
}

func TestExportSupportBundleFailsOnUnreadableWorkspace(t *testing.T) {
	r := testRolle(t)
	if err := os.WriteFile(r.svc.WorkspacePath, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ExportSupportBundle(); err == nil {
		t.Fatal("bundle written from a corrupt workspace")
	}
}

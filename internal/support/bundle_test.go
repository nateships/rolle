package support

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// readZip returns the files in a zip by name.
func readZip(t *testing.T, data []byte) map[string]string {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		files[f.Name] = string(b)
	}
	return files
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

// fakeHome points HOME at an empty directory and clears the cloud tool
// overrides, so no test reads the real profile files.
func fakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("AZURE_CONFIG_DIR", "")
	t.Setenv("CLOUDSDK_CONFIG", "")
	t.Setenv("PATH", t.TempDir())
	return home
}

func TestCloudToolsReportsProfilesAndWithholdsADC(t *testing.T) {
	fakeHome(t)
	azDir, gcDir := t.TempDir(), t.TempDir()
	t.Setenv("AZURE_CONFIG_DIR", azDir)
	t.Setenv("CLOUDSDK_CONFIG", gcDir)
	writeFile(t, filepath.Join(azDir, "azureProfile.json"), `{"subscriptions":[{"id":"0f1e2d3c-4b5a-6978-8a9b-0c1d2e3f4a5b","user":{"name":"nate@contoso.com"}}]}`)
	writeFile(t, filepath.Join(azDir, "config"), "[core]\noutput = json\n")
	writeFile(t, filepath.Join(gcDir, "active_config"), "work\n")
	writeFile(t, filepath.Join(gcDir, "configurations", "config_work"), "[core]\naccount = nate@example.com\nproject = data-platform\n")
	writeFile(t, filepath.Join(gcDir, "configurations", "config_other"), "[core]\nproject = other-project\n")
	writeFile(t, filepath.Join(gcDir, "application_default_credentials.json"), `{"refresh_token":"1//super-secret-refresh"}`)

	out := cloudTools()
	for _, want := range []string{
		"az: not on PATH\n", "gcloud: not on PATH\n", "aws: not on PATH\n",
		"--- " + filepath.Join(azDir, "azureProfile.json") + " ---\n",
		"output = json",
		"--- " + filepath.Join(gcDir, "active_config") + " ---\nwork\n",
		"--- " + filepath.Join(gcDir, "configurations", "config_work") + " ---\n",
		"project = data-platform",
		filepath.Join(gcDir, "application_default_credentials.json") + ": present (contents withheld)\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("cloudTools lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "super-secret-refresh") {
		t.Fatal("application default credentials were copied into the bundle")
	}
	if strings.Contains(out, "other-project") {
		t.Fatal("an inactive gcloud configuration was included")
	}

	// The bundle applies redaction to the same text.
	var buf bytes.Buffer
	if err := Write(&buf, Inputs{Version: "0.1", App: "cli"}); err != nil {
		t.Fatal(err)
	}
	clouds := readZip(t, buf.Bytes())["clouds.txt"]
	for _, leak := range []string{"nate@contoso.com", "nate@example.com", "0f1e2d3c-4b5a-6978-8a9b-0c1d2e3f4a5b"} {
		if strings.Contains(clouds, leak) {
			t.Errorf("clouds.txt leaks %q", leak)
		}
	}
	if !strings.Contains(clouds, "<email>") || !strings.Contains(clouds, "<guid>") || !strings.Contains(clouds, "project = data-platform") {
		t.Fatalf("clouds.txt:\n%s", clouds)
	}
}

func TestCloudToolsDefaultsToHomeDirectories(t *testing.T) {
	home := fakeHome(t)
	writeFile(t, filepath.Join(home, ".azure", "config"), "[core]\ncollect_telemetry = no\n")
	writeFile(t, filepath.Join(home, ".config", "gcloud", "active_config"), "default")
	bin := t.TempDir()
	az := filepath.Join(bin, "az")
	if runtime.GOOS == "windows" {
		az += ".exe"
	}
	writeFile(t, az, "#!/bin/sh\n")
	if err := os.Chmod(az, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	out := cloudTools()
	adc := filepath.Join(home, ".config", "gcloud", "application_default_credentials.json")
	for _, want := range []string{
		"az: " + az + "\n",
		"gcloud: not on PATH\n",
		"collect_telemetry = no",
		"--- " + filepath.Join(home, ".config", "gcloud", "active_config") + " ---\ndefault\n",
		adc + ": absent\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("cloudTools lacks %q:\n%s", want, out)
		}
	}
	// The active configuration names a file that does not exist. It is skipped.
	if strings.Contains(out, "config_default") {
		t.Fatalf("missing configuration listed:\n%s", out)
	}
}

func TestDefaultPathPrefersDownloads(t *testing.T) {
	home := fakeHome(t)
	now := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Setenv("TMP", tmp)
	t.Setenv("TEMP", tmp)

	got := DefaultPath(now)
	if filepath.Base(got) != "rolle-support-20260304-050607.zip" {
		t.Fatalf("name = %q", filepath.Base(got))
	}
	if filepath.Dir(got) != filepath.Clean(os.TempDir()) {
		t.Fatalf("without Downloads: dir = %q, want the temp dir %q", filepath.Dir(got), os.TempDir())
	}

	// A file named Downloads does not count.
	writeFile(t, filepath.Join(home, "Downloads"), "")
	if got := DefaultPath(now); filepath.Dir(got) == filepath.Join(home, "Downloads") {
		t.Fatal("a plain file was taken for the Downloads folder")
	}
	if err := os.Remove(filepath.Join(home, "Downloads")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(home, "Downloads"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got := DefaultPath(now); got != filepath.Join(home, "Downloads", "rolle-support-20260304-050607.zip") {
		t.Fatalf("with Downloads: %q", got)
	}
}

func TestWriteFileCreatesParentsAndRestrictsMode(t *testing.T) {
	fakeHome(t)
	path := filepath.Join(t.TempDir(), "nested", "deeper", "bundle.zip")
	if err := WriteFile(path, Inputs{Version: "1.2.3", App: "desktop", Log: []string{"one", "two"}}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	files := readZip(t, data)
	if files["log.txt"] != "one\ntwo\n" {
		t.Fatalf("log.txt = %q", files["log.txt"])
	}
	if !strings.Contains(files["README.txt"], "desktop 1.2.3") {
		t.Fatalf("README.txt = %q", files["README.txt"])
	}
	if !strings.Contains(files["info.json"], `"app": "desktop"`) || !strings.Contains(files["info.json"], `"version": "1.2.3"`) {
		t.Fatalf("info.json = %s", files["info.json"])
	}
	if _, ok := files["workspace.json"]; ok {
		t.Fatal("workspace.json written without a workspace")
	}
	if _, ok := files["aws-config.txt"]; ok {
		t.Fatal("aws-config.txt written without a config path")
	}

	// A second write replaces the first file instead of appending to it.
	if err := WriteFile(path, Inputs{Version: "2.0.0", App: "cli"}); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if got := readZip(t, data)["README.txt"]; !strings.Contains(got, "cli 2.0.0") {
		t.Fatalf("README.txt after rewrite = %q", got)
	}
}

func TestWriteFileFailsWhenParentIsAFile(t *testing.T) {
	fakeHome(t)
	blocker := filepath.Join(t.TempDir(), "file")
	writeFile(t, blocker, "")
	if err := WriteFile(filepath.Join(blocker, "bundle.zip"), Inputs{}); err == nil {
		t.Fatal("expected an error when the parent is a file")
	}
}

func TestWriteRedactsAWSConfigAndSkipsMissingOne(t *testing.T) {
	fakeHome(t)
	cfg := filepath.Join(t.TempDir(), "config")
	writeFile(t, cfg, "[profile prod]\nsso_start_url = https://acme.awsapps.com/start\nsso_account_id = 123456789012\nsso_role_name = Admin\ncredential_process = /opt/rolle creds --session abc\n")
	var buf bytes.Buffer
	if err := Write(&buf, Inputs{AWSConfigPath: cfg}); err != nil {
		t.Fatal(err)
	}
	files := readZip(t, buf.Bytes())
	got, ok := files["aws-config.txt"]
	if !ok {
		t.Fatal("aws-config.txt missing")
	}
	if strings.Contains(got, "123456789012") || strings.Contains(got, "acme.awsapps") {
		t.Fatalf("aws-config.txt leaks identifiers:\n%s", got)
	}
	if !strings.Contains(got, "sso_account_id = <account>") || !strings.Contains(got, "https://<org>.awsapps.com/start") || !strings.Contains(got, "sso_role_name = Admin") {
		t.Fatalf("aws-config.txt:\n%s", got)
	}
	if !strings.Contains(files["info.json"], `"awsConfigPath": "`+strings.ReplaceAll(cfg, `\`, `\\`)+`"`) {
		t.Fatalf("info.json lacks the config path:\n%s", files["info.json"])
	}

	buf.Reset()
	if err := Write(&buf, Inputs{AWSConfigPath: filepath.Join(t.TempDir(), "absent")}); err != nil {
		t.Fatal(err)
	}
	if _, ok := readZip(t, buf.Bytes())["aws-config.txt"]; ok {
		t.Fatal("aws-config.txt written for a missing file")
	}
}

func TestRedactSecretsAfterSeparators(t *testing.T) {
	cases := map[string]string{
		"client_secret=abc123":       "client_secret=<redacted>",
		"Password: hunter2":          "Password: <redacted>",
		"refresh token : xyz.123":    "refresh token : <redacted>",
		"tokenExpires: 2026-01-01":   "tokenExpires: <redacted>",
		"role/Admin stays":           "role/Admin stays",
		"key ASIAABCDEFGHIJKLMNOP x": "key <access-key-id> x",
		"arn:aws:iam::12345678901:x": "arn:aws:iam::12345678901:x",
	}
	for in, want := range cases {
		if got := Redact(in); got != want {
			t.Errorf("Redact(%q) = %q, want %q", in, got, want)
		}
	}
}

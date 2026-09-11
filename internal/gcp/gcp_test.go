package gcp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteImpersonatedADC(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "adc.json")
	if err := os.WriteFile(src, []byte(`{"type":"authorized_user","client_id":"c","client_secret":"s","refresh_token":"r"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", src)
	out := filepath.Join(dir, "gcp", "sess.json")
	if err := WriteImpersonatedADC(out, "sa@p.iam.gserviceaccount.com"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["type"] != "impersonated_service_account" || !strings.Contains(got["service_account_impersonation_url"].(string), "sa@p.iam.gserviceaccount.com") {
		t.Fatalf("adc = %v", got)
	}
	if os.WriteFile(src, []byte(`{"type":"service_account"}`), 0o600) != nil {
		t.Fatal("rewrite")
	}
	if err := WriteImpersonatedADC(out, "sa@p.iam.gserviceaccount.com"); err == nil {
		t.Fatal("expected refusal for service_account source")
	}
}

func TestTokenSourceRejectsNonUserCredentials(t *testing.T) {
	_, err := tokenSource(context.Background(), "adc.json", []byte(`{"type":"service_account","private_key":"x"}`))
	if err == nil || !strings.Contains(err.Error(), "authorized_user") {
		t.Fatalf("err = %v", err)
	}
	if _, err := tokenSource(context.Background(), "adc.json", []byte(`{"type":"authorized_user","client_id":"a","client_secret":"b","refresh_token":"r"}`)); err != nil {
		t.Fatal(err)
	}
}

func TestConsoleURL(t *testing.T) {
	if ConsoleURL("p1") != "https://console.cloud.google.com/home/dashboard?project=p1" {
		t.Fatal("unexpected console url")
	}
}

func TestIsShimAndSummarize(t *testing.T) {
	if !isShim("/Users/x/.local/share/mise/shims/gcloud") || isShim("/opt/homebrew/bin/gcloud") {
		t.Fatal("shim detection wrong")
	}
	got := summarize("mise WARN something\nmise ERROR No version is set for shim: gcloud\nSet a global default", errors.New("exit 1"))
	if !strings.Contains(got, "No version is set") {
		t.Fatalf("summarize = %q", got)
	}
	if summarize("", errors.New("exit 1")) != "exit 1" {
		t.Fatal("empty stderr should fall back to the error")
	}
}

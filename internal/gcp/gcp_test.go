package gcp

import (
	"context"
	"encoding/json"
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

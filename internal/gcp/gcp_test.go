package gcp

import (
	"context"
	"strings"
	"testing"
)

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

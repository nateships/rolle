package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"testing"
)

func TestParsePublicKeyPEMAndBase64(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	pemText := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	got, err := parsePublicKey(pemText)
	if err != nil || string(got) != string(pub) {
		t.Fatalf("pem: %v", err)
	}
	got, err = parsePublicKey(base64.StdEncoding.EncodeToString(pub))
	if err != nil || string(got) != string(pub) {
		t.Fatalf("base64: %v", err)
	}
	if _, err := parsePublicKey(""); err == nil {
		t.Fatal("empty key accepted")
	}
	if _, err := parsePublicKey(updaterPublicKey); err != nil {
		t.Fatalf("embedded key: %v", err)
	}
}

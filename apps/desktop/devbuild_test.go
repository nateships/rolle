package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"strings"
	"testing"

	"github.com/nateships/rolle/internal/version"
)

func TestIsDevBuild(t *testing.T) {
	prev := version.Version
	t.Cleanup(func() { version.Version = prev })
	cases := map[string]bool{
		"":            true,
		"0.0.1-dev":   true,
		"1.2.3-dev.4": true,
		"v1.2.3":      false,
		"1.2.3":       false,
		"1.2.3-rc.1":  false,
	}
	for v, want := range cases {
		version.Version = v
		if got := isDevBuild(); got != want {
			t.Errorf("isDevBuild() with %q = %v, want %v", v, got, want)
		}
	}
}

func TestParsePublicKeyRejectsBadInput(t *testing.T) {
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ecDER, err := x509.MarshalPKIXPublicKey(&ecKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"whitespace only", " \n\t", "empty"},
		{"not base64", "*** not base64 ***", "illegal base64"},
		{"wrong length", base64.StdEncoding.EncodeToString(make([]byte, 16)), "wrong length"},
		{"pem with ecdsa key", string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: ecDER})), "not Ed25519"},
		{"pem with garbage", string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte("garbage")})), "asn1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parsePublicKey(tc.in)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestUpdaterCallsWithoutAppAreNoops(t *testing.T) {
	if !isDevBuild() {
		t.Skip("tests run against the dev version")
	}
	r := &RolleService{}
	info, err := r.CheckForUpdates()
	if err != nil {
		t.Fatal(err)
	}
	if info.Enabled || info.Available || info.State != "disabled" || info.CurrentVersion != version.Version {
		t.Fatalf("info = %+v", info)
	}
	if err := r.InstallUpdate(); err != nil {
		t.Fatal(err)
	}
}

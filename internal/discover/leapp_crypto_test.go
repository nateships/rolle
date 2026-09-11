package discover

import (
	"encoding/base64"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"
)

// Reference values from:
// openssl enc -aes-256-cbc -k secret -S 0102030405060708 -P -md md5
func TestEvpBytesToKeyMatchesOpenSSL(t *testing.T) {
	salt, _ := hex.DecodeString("0102030405060708")
	key, iv := evpBytesToKey([]byte("secret"), salt, 32, 16)
	if got := hex.EncodeToString(key); got != "c9e5a1bd216dbe1317e230cef48f38ee7f0e17ad64022144bccec4a1aa2879ab" {
		t.Fatalf("key = %s", got)
	}
	if got := hex.EncodeToString(iv); got != "e24b32bbbc4ef02ecbcb6576523ad893" {
		t.Fatalf("iv = %s", got)
	}
}

func TestDecryptCryptoJSRejectsMalformed(t *testing.T) {
	salt := []byte("12345678")
	b64 := func(b ...[]byte) string {
		var all []byte
		for _, p := range b {
			all = append(all, p...)
		}
		return base64.StdEncoding.EncodeToString(all)
	}
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"not base64", "!!!not base64!!!", "illegal base64"},
		{"too short", b64([]byte("Salted_")), "not an OpenSSL salted payload"},
		{"wrong magic", b64([]byte("Unsalted"), salt, make([]byte, 16)), "not an OpenSSL salted payload"},
		{"empty body", b64([]byte("Salted__"), salt), "not block aligned"},
		{"unaligned body", b64([]byte("Salted__"), salt, make([]byte, 15)), "not block aligned"},
		{"garbage ciphertext", b64([]byte("Salted__"), salt, make([]byte, 16)), "bad padding"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decryptCryptoJS(tc.in, "pass")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestEncryptDecryptRoundTripsEveryPaddingLength(t *testing.T) {
	for _, n := range []int{0, 1, 15, 16, 17, 31, 32, 100} {
		plain := []byte(strings.Repeat("x", n))
		enc := encryptCryptoJS(plain, "pass", []byte("abcdefgh"))
		got, err := decryptCryptoJS(enc, "pass")
		if err != nil || string(got) != string(plain) {
			t.Fatalf("len %d: got %q, %v", n, got, err)
		}
	}
}

func TestParseLeappSkipsIncompleteEntries(t *testing.T) {
	lw, err := parseLeapp([]byte(`{
	  "_sessions":[
	    {"type":"awsIamUser","sessionName":"u","sessionId":"u1","profileId":"p-default"},
	    {"type":"awsIamUser","sessionName":"v","sessionId":"u2","profileId":"unknown"},
	    {"type":"awsIamRoleChained","sessionName":"c","sessionId":"c1","roleArn":"arn","parentSessionId":"gone"},
	    {"type":"awsIamRoleFederated","sessionName":"f","sessionId":"f1"},
	    {"type":"azure","sessionName":"a","sessionId":"a1"}
	  ],
	  "_awsSsoIntegrations":[{"alias":"empty","portalUrl":"","region":"us-east-1"}],
	  "_azureIntegrations":[{"alias":"empty","tenantId":""}],
	  "_profiles":[{"id":"p-default","name":"default"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(lw.Portals) != 0 || len(lw.Tenants) != 0 || lw.SSORoles != 0 {
		t.Fatalf("workspace = %+v", lw)
	}
	if len(lw.IAMUsers) != 2 || lw.IAMUsers[0].Profile != "" || lw.IAMUsers[1].Profile != "" {
		t.Fatalf("users = %+v", lw.IAMUsers)
	}
	if len(lw.ChainedRoles) != 1 || lw.ChainedRoles[0].ParentName != "" || lw.ChainedRoles[0].RoleARN != "arn" {
		t.Fatalf("chained = %+v", lw.ChainedRoles)
	}
}

func TestParseLeappBadJSON(t *testing.T) {
	lw, err := parseLeapp([]byte("{"))
	if err == nil || !strings.Contains(err.Error(), "parse workspace") || lw != nil {
		t.Fatalf("parseLeapp = %+v, %v", lw, err)
	}
}

func TestLeappKeychainKeys(t *testing.T) {
	id, secret := LeappKeychainKeys("u1")
	if id != "u1-iam-user-aws-session-access-key-id" || secret != "u1-iam-user-aws-session-secret-access-key" {
		t.Fatalf("keys = %q, %q", id, secret)
	}
	if LeappKeychainService != "Leapp" {
		t.Fatalf("service = %q", LeappKeychainService)
	}
}

func TestLeappPath(t *testing.T) {
	home := fakeHome(t)
	t.Setenv("LEAPP_HOME", "")
	if got := LeappPath(); got != filepath.Join(home, ".Leapp", "Leapp-lock.json") {
		t.Fatalf("LeappPath = %q", got)
	}
	custom := t.TempDir()
	t.Setenv("LEAPP_HOME", custom)
	if got := LeappPath(); got != filepath.Join(custom, "Leapp-lock.json") {
		t.Fatalf("LeappPath with LEAPP_HOME = %q", got)
	}
}

func TestReadLeapp(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAPP_HOME", dir)
	lw, err := ReadLeapp()
	if err != nil || lw != nil {
		t.Fatalf("missing workspace = %+v, %v", lw, err)
	}
	// A file that is not a crypto-js payload fails, whichever machine id the
	// host reports.
	write(t, filepath.Join(dir, "Leapp-lock.json"), "not a payload\n")
	lw, err = ReadLeapp()
	if err == nil || lw != nil || !strings.Contains(err.Error(), "leapp:") {
		t.Fatalf("garbage workspace = %+v, %v", lw, err)
	}
}

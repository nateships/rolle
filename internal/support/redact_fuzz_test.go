package support

import (
	"regexp"
	"testing"
)

var keyID = regexp.MustCompile(`\b(AKIA|ASIA)[0-9A-Z]{16}\b`)

// FuzzRedact checks the redaction on arbitrary input: it does not panic, it
// is idempotent, and no access key id survives it.
func FuzzRedact(f *testing.F) {
	for _, seed := range []string{
		"",
		"key ASIAABCDEFGHIJKLMNOP x",
		"aws_access_key_id = AKIAIOSFODNN7EXAMPLE",
		"client_secret=abc123",
		"arn:aws:iam::123456789012:role/Admin",
		"nate@example.com https://acme.awsapps.com/start",
		"Password: hunter2\nrefresh token : xyz.123",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, in string) {
		out := Redact(in)
		if again := Redact(out); again != out {
			t.Fatalf("Redact is not idempotent:\n in=%q\nout=%q\nagain=%q", in, out, again)
		}
		if keyID.MatchString(out) {
			t.Fatalf("access key id survives redaction: %q -> %q", in, out)
		}
	})
}

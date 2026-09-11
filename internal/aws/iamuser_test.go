package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"

	"github.com/nateships/rolle/internal/secrets"
)

func TestAccessKeyRoundTrip(t *testing.T) {
	mem := &secrets.Memory{}
	key := AccessKey{AccessKeyID: "AKIA", SecretAccessKey: "s3cr3t"}
	if err := StoreAccessKey(mem, "s1", key); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Get("aws-access-key/s1"); err != nil {
		t.Fatalf("key stored under an unexpected name: %v", err)
	}
	got, err := LoadAccessKey(mem, "s1")
	if err != nil || got != key {
		t.Fatalf("Load = %+v, %v", got, err)
	}
	if _, err := LoadAccessKey(mem, "never"); !errors.Is(err, ErrNoAccessKey) {
		t.Fatalf("unknown session = %v, want ErrNoAccessKey", err)
	}
	if err := DeleteAccessKey(mem, "s1"); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAccessKey(mem, "s1"); !errors.Is(err, ErrNoAccessKey) {
		t.Fatalf("after delete = %v, want ErrNoAccessKey", err)
	}
	if err := DeleteAccessKey(mem, "s1"); err != nil {
		t.Fatalf("second delete: %v", err)
	}
}

func TestLoadAccessKeyErrors(t *testing.T) {
	mem := &secrets.Memory{}
	if err := mem.Set("aws-access-key/s1", "not json"); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAccessKey(mem, "s1"); err == nil || errors.Is(err, ErrNoAccessKey) {
		t.Fatalf("corrupt entry = %v, want a parse error", err)
	}
	boom := errors.New("boom")
	if _, err := LoadAccessKey(failStore{boom}, "s1"); !errors.Is(err, boom) {
		t.Fatalf("store error = %v, want boom", err)
	}
	if err := StoreAccessKey(failStore{boom}, "s1", AccessKey{}); !errors.Is(err, boom) {
		t.Fatalf("store error on Set = %v", err)
	}
}

func TestIAMUserCredentialsWithoutMFAReturnsKey(t *testing.T) {
	key := AccessKey{AccessKeyID: "AKIA", SecretAccessKey: "s3cr3t"}
	cases := []struct {
		name string
		in   IAMUserInput
		want time.Duration
	}{
		{"default duration", IAMUserInput{Key: key}, 12 * time.Hour},
		{"custom duration", IAMUserInput{Key: key, Duration: 30 * time.Minute}, 30 * time.Minute},
		{"negative duration falls back", IAMUserInput{Key: key, Duration: -time.Minute}, 12 * time.Hour},
		{"device without code", IAMUserInput{Key: key, MFADevice: "arn:aws:iam::1:mfa/me"}, 12 * time.Hour},
		{"code without device", IAMUserInput{Key: key, MFACode: "123456"}, 12 * time.Hour},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := time.Now()
			got, err := IAMUserCredentials(context.Background(), tc.in)
			after := time.Now()
			if err != nil {
				t.Fatal(err)
			}
			if got.AccessKeyID != key.AccessKeyID || got.SecretAccessKey != key.SecretAccessKey || got.SessionToken != "" {
				t.Fatalf("creds = %+v", got)
			}
			if got.Expiration == nil || got.Expiration.Location() != time.UTC {
				t.Fatalf("expiration = %v, want a UTC time", got.Expiration)
			}
			if got.Expiration.Before(before.Add(tc.want)) || got.Expiration.After(after.Add(tc.want)) {
				t.Fatalf("expiration = %v, want about %v from now", got.Expiration, tc.want)
			}
		})
	}
}

func TestFromSTS(t *testing.T) {
	if got := fromSTS(nil, nil, nil, nil); got.AccessKeyID != "" || got.SecretAccessKey != "" || got.SessionToken != "" || got.Expiration != nil {
		t.Fatalf("nil inputs = %+v", got)
	}
	loc := time.FixedZone("plus2", 2*3600)
	exp := time.Date(2026, 1, 1, 14, 0, 0, 0, loc)
	got := fromSTS(awssdk.String("id"), awssdk.String("secret"), awssdk.String("token"), &exp)
	if got.AccessKeyID != "id" || got.SecretAccessKey != "secret" || got.SessionToken != "token" {
		t.Fatalf("creds = %+v", got)
	}
	if got.Expiration == nil || !got.Expiration.Equal(exp) || got.Expiration.Location() != time.UTC {
		t.Fatalf("expiration = %v, want %v in UTC", got.Expiration, exp)
	}
}

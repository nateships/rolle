package aws

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nateships/rolle/internal/core"
)

// isolateAWS keeps the SDK away from the real shared config and disables retries.
func isolateAWS(t *testing.T) {
	t.Helper()
	missing := filepath.Join(t.TempDir(), "absent")
	t.Setenv("AWS_CONFIG_FILE", missing)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", missing)
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_MAX_ATTEMPTS", "1")
}

// fakeSTS answers the Query protocol calls the package makes.
type fakeSTS struct {
	t     *testing.T
	forms []map[string]string
	fail  string // error code to return, empty for success
}

func (f *fakeSTS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		f.t.Errorf("parse form: %v", err)
	}
	form := map[string]string{}
	for k := range r.PostForm {
		form[k] = r.PostForm.Get(k)
	}
	f.forms = append(f.forms, form)
	w.Header().Set("Content-Type", "text/xml")
	if f.fail != "" {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, `<ErrorResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><Error><Type>Sender</Type><Code>%s</Code><Message>refused by test</Message></Error><RequestId>r1</RequestId></ErrorResponse>`, f.fail)
		return
	}
	action := form["Action"]
	fmt.Fprintf(w, `<%sResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><%sResult><Credentials><AccessKeyId>ASIATEST</AccessKeyId><SecretAccessKey>sts-secret</SecretAccessKey><SessionToken>sts-token</SessionToken><Expiration>2026-01-01T13:00:00Z</Expiration></Credentials></%sResult><ResponseMetadata><RequestId>r1</RequestId></ResponseMetadata></%sResponse>`, action, action, action, action)
}

func startFakeSTS(t *testing.T) *fakeSTS {
	t.Helper()
	isolateAWS(t)
	f := &fakeSTS{t: t}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	t.Setenv("AWS_ENDPOINT_URL_STS", srv.URL)
	return f
}

var wantSTSExp = time.Date(2026, 1, 1, 13, 0, 0, 0, time.UTC)

func TestAssumeRoleSendsEveryOptionalField(t *testing.T) {
	sts := startFakeSTS(t)
	got, err := AssumeRole(context.Background(), AssumeRoleInput{
		Source:      core.Credentials{AccessKeyID: "AKIA", SecretAccessKey: "s", SessionToken: "src-token"},
		Region:      "eu-west-1",
		RoleARN:     "arn:aws:iam::123456789012:role/Admin",
		SessionName: "rolle-admin",
		ExternalID:  "ext-1",
		Duration:    30 * time.Minute,
		MFADevice:   "arn:aws:iam::123456789012:mfa/me",
		MFACode:     "123456",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessKeyID != "ASIATEST" || got.SecretAccessKey != "sts-secret" || got.SessionToken != "sts-token" || got.Expiration == nil || !got.Expiration.Equal(wantSTSExp) {
		t.Fatalf("creds = %+v", got)
	}
	if len(sts.forms) != 1 {
		t.Fatalf("%d requests", len(sts.forms))
	}
	form := sts.forms[0]
	want := map[string]string{
		"Action": "AssumeRole", "RoleArn": "arn:aws:iam::123456789012:role/Admin", "RoleSessionName": "rolle-admin",
		"DurationSeconds": "1800", "ExternalId": "ext-1", "SerialNumber": "arn:aws:iam::123456789012:mfa/me", "TokenCode": "123456",
	}
	for k, v := range want {
		if form[k] != v {
			t.Errorf("form[%s] = %q, want %q", k, form[k], v)
		}
	}
}

func TestAssumeRoleDefaultsAndOmitsUnsetFields(t *testing.T) {
	sts := startFakeSTS(t)
	_, err := AssumeRole(context.Background(), AssumeRoleInput{
		Source:    core.Credentials{AccessKeyID: "AKIA", SecretAccessKey: "s"},
		Region:    "us-east-1",
		RoleARN:   "arn:aws:iam::1:role/x",
		MFADevice: "arn:aws:iam::1:mfa/me", // no code, so MFA is not sent
	})
	if err != nil {
		t.Fatal(err)
	}
	form := sts.forms[0]
	if form["DurationSeconds"] != "3600" || form["RoleSessionName"] != "rolle" {
		t.Fatalf("defaults: %v", form)
	}
	for _, absent := range []string{"ExternalId", "SerialNumber", "TokenCode"} {
		if _, ok := form[absent]; ok {
			t.Errorf("%s sent although unset", absent)
		}
	}
}

func TestAssumeRoleReportsRefusal(t *testing.T) {
	sts := startFakeSTS(t)
	sts.fail = "AccessDenied"
	_, err := AssumeRole(context.Background(), AssumeRoleInput{Source: core.Credentials{AccessKeyID: "A", SecretAccessKey: "B"}, Region: "us-east-1", RoleARN: "arn:aws:iam::1:role/x"})
	if err == nil || !strings.Contains(err.Error(), "assume role arn:aws:iam::1:role/x") || !strings.Contains(err.Error(), "AccessDenied") {
		t.Fatalf("err = %v", err)
	}
}

func TestIAMUserCredentialsWithMFACallsGetSessionToken(t *testing.T) {
	sts := startFakeSTS(t)
	got, err := IAMUserCredentials(context.Background(), IAMUserInput{
		Key: AccessKey{AccessKeyID: "AKIA", SecretAccessKey: "s"}, Region: "us-east-1",
		MFADevice: "arn:aws:iam::1:mfa/me", MFACode: "654321", Duration: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessKeyID != "ASIATEST" || got.SessionToken != "sts-token" || got.Expiration == nil || !got.Expiration.Equal(wantSTSExp) {
		t.Fatalf("creds = %+v", got)
	}
	form := sts.forms[0]
	if form["Action"] != "GetSessionToken" || form["SerialNumber"] != "arn:aws:iam::1:mfa/me" || form["TokenCode"] != "654321" || form["DurationSeconds"] != "3600" {
		t.Fatalf("form = %v", form)
	}
	// The long-lived key signs the request; it is never sent in the body.
	for k, v := range form {
		if strings.Contains(k, "Secret") || v == "s" {
			t.Fatalf("secret key in form: %v", form)
		}
	}

	sts.fail = "InvalidClientTokenId"
	_, err = IAMUserCredentials(context.Background(), IAMUserInput{Key: AccessKey{AccessKeyID: "A", SecretAccessKey: "B"}, Region: "us-east-1", MFADevice: "d", MFACode: "1"})
	if err == nil || !strings.Contains(err.Error(), "get session token") || !strings.Contains(err.Error(), "InvalidClientTokenId") {
		t.Fatalf("err = %v", err)
	}
}

func TestStoreAccessKeyPropagatesStoreError(t *testing.T) {
	boom := errors.New("boom")
	if err := StoreAccessKey(failStore{boom}, "s1", AccessKey{AccessKeyID: "a"}); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

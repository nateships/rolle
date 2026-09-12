package gcp

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// anonymousADC has no account field, so the identity needs a lookup.
const anonymousADC = `{"type":"authorized_user","client_id":"c","client_secret":"s","refresh_token":"r"}`

// cancelled returns a context that is already done, so any network call
// fails at once.
func cancelled() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestUnreadableADCIsNotTreatedAsMissing(t *testing.T) {
	_, path := fakeHome(t)
	if err := os.MkdirAll(path, 0o700); err != nil { // a directory where the file should be
		t.Fatal(err)
	}
	if _, err := DetectAccount(context.Background()); err == nil || errors.Is(err, ErrNoADC) {
		t.Fatalf("DetectAccount = %v, want a read error", err)
	}
	if _, err := SourceToken(context.Background()); err == nil || errors.Is(err, ErrNoADC) {
		t.Fatalf("SourceToken = %v, want a read error", err)
	}
	if err := WriteImpersonatedADC(filepath.Join(t.TempDir(), "out.json"), "sa"); err == nil || errors.Is(err, ErrNoADC) {
		t.Fatalf("WriteImpersonatedADC = %v, want a read error", err)
	}
}

func TestDetectAccountWithoutAccountFieldSkipsLookupOffline(t *testing.T) {
	_, path := fakeHome(t)
	writeADC(t, path, anonymousADC)
	acct, err := DetectAccount(cancelled())
	if err != nil {
		t.Fatal(err)
	}
	if acct.Email != "" {
		t.Fatalf("email = %q, want empty when the lookup fails", acct.Email)
	}
	writeADC(t, path, "{not json")
	if _, err := DetectAccount(context.Background()); err == nil || !strings.Contains(err.Error(), "parse "+path) {
		t.Fatalf("corrupt ADC = %v", err)
	}
}

func TestUserEmailErrors(t *testing.T) {
	if _, err := userEmail(context.Background(), "adc.json", []byte(`{"type":"service_account"}`)); err == nil || !strings.Contains(err.Error(), "authorized_user") {
		t.Fatalf("service account = %v", err)
	}
	if _, err := userEmail(cancelled(), "adc.json", []byte(anonymousADC)); err == nil {
		t.Fatal("expected a failure with a cancelled context")
	}
}

func TestSourceTokenFailsWhenTheTokenEndpointIsUnreachable(t *testing.T) {
	_, path := fakeHome(t)
	writeADC(t, path, userADC)
	if _, err := SourceToken(cancelled()); err == nil || errors.Is(err, ErrNoADC) {
		t.Fatalf("SourceToken = %v, want a network error", err)
	}
}

func TestWriteImpersonatedADCRejectsBadSource(t *testing.T) {
	_, path := fakeHome(t)
	out := filepath.Join(t.TempDir(), "gcp", "s1.json")
	writeADC(t, path, "{not json")
	if err := WriteImpersonatedADC(out, "sa"); err == nil || !strings.Contains(err.Error(), "parse "+path) {
		t.Fatalf("corrupt source = %v", err)
	}
	writeADC(t, path, `{"type":"external_account"}`)
	if err := WriteImpersonatedADC(out, "sa"); err == nil || !strings.Contains(err.Error(), "external_account") {
		t.Fatalf("external account source = %v", err)
	}
	if _, err := os.Stat(out); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a file was written for a rejected source")
	}
	writeADC(t, path, userADC)
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteImpersonatedADC(filepath.Join(blocker, "s1.json"), "sa"); err == nil {
		t.Fatal("expected an error when the parent is a file")
	}
}

// errTransport fails every request.
type errTransport struct{ err error }

func (e errTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, e.err }

func TestTransportErrorsPropagate(t *testing.T) {
	boom := errors.New("boom")
	client := &http.Client{Transport: errTransport{boom}}
	if _, err := ListProjects(context.Background(), client, "tok"); !errors.Is(err, boom) {
		t.Fatalf("ListProjects = %v", err)
	}
	if _, err := Impersonate(context.Background(), client, "tok", "sa", 0); !errors.Is(err, boom) {
		t.Fatalf("Impersonate = %v", err)
	}
}

func TestGCloudLoginRunsTheCLI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fakes need a POSIX shell")
	}
	fakeHome(t)
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	gcloud := filepath.Join(bin, "gcloud")
	args := filepath.Join(bin, "args")
	script := "#!/bin/sh\necho \"$@\" > " + args + "\nexit 0\n"
	if err := os.WriteFile(gcloud, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := GCloudLogin(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(args)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(got)) != "auth application-default login --quiet" {
		t.Fatalf("gcloud args = %q", got)
	}

	script = "#!/bin/sh\necho 'WARNING: something' >&2\necho 'ERROR: (gcloud.auth) no browser' >&2\nexit 1\n"
	if err := os.WriteFile(gcloud, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	err = GCloudLogin(context.Background())
	if err == nil || err.Error() != "gcloud login: ERROR: (gcloud.auth) no browser" {
		t.Fatalf("failing gcloud = %v", err)
	}

	t.Setenv("PATH", t.TempDir())
	// A system install at a well-known path is still found. Skip there.
	for _, c := range candidates() {
		if _, err := os.Stat(c); err == nil {
			t.Skipf("gcloud installed at %s", c)
		}
	}
	if err := GCloudLogin(context.Background()); !errors.Is(err, ErrGCloudMissing) {
		t.Fatalf("missing gcloud = %v", err)
	}
}

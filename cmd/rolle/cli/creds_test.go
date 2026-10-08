package cli

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nateships/rolle/internal/app"
	"github.com/nateships/rolle/internal/aws"
	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/netcfg"
)

func TestCredsStopsWhenTheProviderStalls(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent")
	t.Setenv("AWS_CONFIG_FILE", missing)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", missing)
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_MAX_ATTEMPTS", "1")
	// The STS stub accepts the request and never answers.
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	// Cleanups run last in, first out: release the handler before Close waits for it.
	t.Cleanup(func() { close(release) })
	t.Setenv("AWS_ENDPOINT_URL_STS", srv.URL)

	old := credsTimeout
	credsTimeout = 200 * time.Millisecond
	t.Cleanup(func() { credsTimeout = old })

	s := testCLI(t)
	if _, err := s.AddIAMUser(app.AddIAMUserInput{Name: "src", Region: "us-east-1", Profile: "src", Key: aws.AccessKey{AccessKeyID: "AKIA", SecretAccessKey: "secret"}}); err != nil {
		t.Fatal(err)
	}
	role, err := s.AddAssumeRole(app.AddAssumeRoleInput{Name: "admin", Region: "us-east-1", RoleARN: "arn:aws:iam::1:role/admin", SourceRef: "src"})
	if err != nil {
		t.Fatal(err)
	}
	w, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	for i := range w.Sessions {
		w.Sessions[i].Status = core.StatusActive
	}
	if err := s.Save(w); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := run(t, "creds", "--session", role.ID)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("creds = %v, want a deadline error", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("creds still waits for a stalled provider")
	}
}

func TestTokenCommandsStopWhenTheProviderStalls(t *testing.T) {
	// The ADC file gives the GCP session a refresh token to exchange.
	adc := filepath.Join(t.TempDir(), "adc.json")
	if err := os.WriteFile(adc, []byte(`{"type":"authorized_user","client_id":"cid","client_secret":"cs","refresh_token":"rt"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", adc)
	t.Setenv("AWS_CA_BUNDLE", "")
	// The proxy stub accepts the request and never answers.
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	// Cleanups run last in, first out: release the handler before Close waits for it.
	t.Cleanup(func() { close(release) })
	if err := netcfg.Apply(core.Settings{ProxyURL: srv.URL}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = netcfg.Apply(core.Settings{}) })

	old := credsTimeout
	credsTimeout = 200 * time.Millisecond
	t.Cleanup(func() { credsTimeout = old })

	s := testCLI(t)
	w, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	w.Sessions = append(w.Sessions, core.Session{ID: "g1", Name: "proj", Kind: core.KindGCP, Status: core.StatusActive, GCP: &core.GCPSession{ProjectID: "proj"}})
	if err := s.Save(w); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"token", "proj"},
		{"kube", "token", "proj"},
		{"env", "proj"},
		{"env", "proj", "--profile"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			done := make(chan error, 1)
			go func() {
				_, err := run(t, args...)
				done <- err
			}()
			select {
			case err := <-done:
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("%v = %v, want a deadline error", args, err)
				}
			case <-time.After(10 * time.Second):
				t.Fatalf("%v still waits for a stalled provider", args)
			}
		})
	}
}

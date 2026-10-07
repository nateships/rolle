package cli

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/nateships/rolle/internal/app"
	"github.com/nateships/rolle/internal/aws"
	"github.com/nateships/rolle/internal/core"
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

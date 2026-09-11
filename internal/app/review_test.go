package app

import (
	"context"
	"strings"
	"testing"

	"github.com/nateships/rolle/internal/aws"
	"github.com/nateships/rolle/internal/core"
)

func TestRoleSessionNameFitsSTSLimit(t *testing.T) {
	long := roleSessionName(strings.Repeat("Production Administrator ", 4))
	if len(long) != 64 || !strings.HasPrefix(long, "rolle-Production-Administrator") {
		t.Fatalf("roleSessionName = %q (%d chars)", long, len(long))
	}
	if got := roleSessionName("dev"); got != "rolle-dev" {
		t.Fatalf("roleSessionName = %q", got)
	}
}

// A dependent listed before its source must still go when the integration goes.
func TestRemoveIntegrationDropsDependentsListedBeforeTheirSource(t *testing.T) {
	s := testService(t)
	in, err := s.AddAWSSSO("acme", "https://acme.awsapps.com/start", "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	w, _ := s.Load()
	role := func(id, src string) core.Session {
		return core.Session{ID: id, Name: id, Kind: core.KindAWSAssumeRole, AWS: &core.AWSSession{SourceSessionID: src, RoleARN: "arn:aws:iam::1:role/x"}}
	}
	sso := func(id string) core.Session {
		return core.Session{ID: id, Name: id, Kind: core.KindAWSSSORole, IntegrationID: in.ID, AWS: &core.AWSSession{AccountID: "1", RoleName: id}}
	}
	w.Sessions = append(w.Sessions, role("depA", "A"), sso("A"), sso("B"), role("depB", "B"))
	if err := s.Save(w); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveIntegration("acme"); err != nil {
		t.Fatal(err)
	}
	w, _ = s.Load()
	if len(w.Sessions) != 0 {
		t.Fatalf("sessions left after remove: %+v", w.Sessions)
	}
}

func TestStartRefusesRoleThatSharesProfileWithSource(t *testing.T) {
	s := testService(t)
	if _, err := s.AddIAMUser(AddIAMUserInput{Name: "src", Region: "us-east-1", Key: aws.AccessKey{AccessKeyID: "A", SecretAccessKey: "B"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Start(context.Background(), "src", StartOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddAssumeRole(AddAssumeRoleInput{Name: "admin", Region: "us-east-1", RoleARN: "arn:aws:iam::1:role/admin", SourceRef: "src"}); err != nil {
		t.Fatal(err)
	}
	_, err := s.Start(context.Background(), "admin", StartOptions{})
	if err == nil || !strings.Contains(err.Error(), `both write AWS profile "default"`) {
		t.Fatalf("Start(admin) = %v, want a shared-profile refusal", err)
	}
	w, _ := s.Load()
	if src, _ := w.Session(mustFind(t, w, "src")); src.Status != core.StatusActive {
		t.Fatal("the refusal must leave the source running")
	}
}

func mustFind(t *testing.T, w *core.Workspace, ref string) string {
	t.Helper()
	sess, err := FindSession(w, ref)
	if err != nil {
		t.Fatal(err)
	}
	return sess.ID
}

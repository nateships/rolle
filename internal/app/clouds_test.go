package app

import (
	"errors"
	"testing"

	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/discover"
)

func TestAddAzureAndImpersonationSessions(t *testing.T) {
	s := testService(t)
	in, err := s.AddAzure("contoso", "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if in.Cloud != core.CloudAzure || in.Azure.TenantID != "tenant-1" {
		t.Fatalf("integration = %+v", in)
	}
	if _, err := s.AddGCPImpersonation(AddGCPImpersonationInput{Name: "x", IntegrationRef: "contoso", ProjectID: "p", ServiceAccount: "sa@p.iam.gserviceaccount.com"}); err == nil {
		t.Fatal("expected refusal to impersonate through an Azure integration")
	}
	w, _ := s.Load()
	w.Integrations = append(w.Integrations, core.Integration{ID: "g1", Alias: "gcp", Cloud: core.CloudGCP, GCP: &core.GCPIntegration{Account: "me@example.com"}})
	if err := s.Save(w); err != nil {
		t.Fatal(err)
	}
	sess, err := s.AddGCPImpersonation(AddGCPImpersonationInput{Name: "deployer", IntegrationRef: "gcp", ProjectID: "p", ServiceAccount: "sa@p.iam.gserviceaccount.com"})
	if err != nil {
		t.Fatal(err)
	}
	if sess.Kind != core.KindGCP || sess.GCP.ServiceAccount != "sa@p.iam.gserviceaccount.com" {
		t.Fatalf("session = %+v", sess)
	}
	rejected := map[string]AddGCPImpersonationInput{
		"duplicate name":  {Name: "deployer", IntegrationRef: "gcp", ProjectID: "p", ServiceAccount: "sa@p.iam.gserviceaccount.com"},
		"empty name":      {Name: " ", IntegrationRef: "gcp", ProjectID: "p", ServiceAccount: "sa@p.iam.gserviceaccount.com"},
		"empty project":   {Name: "other", IntegrationRef: "gcp", ServiceAccount: "sa@p.iam.gserviceaccount.com"},
		"empty principal": {Name: "other", IntegrationRef: "gcp", ProjectID: "p"},
	}
	for name, in := range rejected {
		if _, err := s.AddGCPImpersonation(in); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if w, _ := s.Load(); len(w.Sessions) != 1 {
		t.Fatalf("rejected input must not add sessions: %+v", w.Sessions)
	}
}

func TestEnvVarsPerCloud(t *testing.T) {
	aws := &core.Session{Kind: core.KindAWSSSORole, Region: "eu-west-1"}
	got := EnvVars(aws, core.Credentials{AccessKeyID: "A", SecretAccessKey: "S", SessionToken: "T"})
	if got[0][0] != "AWS_ACCESS_KEY_ID" || got[0][1] != "A" || got[3][1] != "eu-west-1" {
		t.Fatalf("aws vars = %v", got)
	}
	az := &core.Session{Kind: core.KindAzure, Azure: &core.AzureSession{SubscriptionID: "sub", TenantID: "ten"}}
	got = EnvVars(az, core.Credentials{Token: "tok"})
	if got[0][1] != "sub" || got[4][0] != "AZURE_ACCESS_TOKEN" || got[4][1] != "tok" {
		t.Fatalf("azure vars = %v", got)
	}
	g := &core.Session{Kind: core.KindGCP, GCP: &core.GCPSession{ProjectID: "proj", ServiceAccount: "sa@x"}}
	got = EnvVars(g, core.Credentials{Token: "tok"})
	if got[0][1] != "proj" || got[len(got)-1][0] != "CLOUDSDK_AUTH_IMPERSONATE_SERVICE_ACCOUNT" {
		t.Fatalf("gcp vars = %v", got)
	}
	if EnvVars(&core.Session{Kind: core.Kind("bogus")}, core.Credentials{}) != nil {
		t.Fatal("expected nil for unknown kind")
	}
}

func TestImportLeappSessions(t *testing.T) {
	s := testService(t)
	keys := map[string]string{"u1-iam-user-aws-session-access-key-id": "AKIA", "u1-iam-user-aws-session-secret-access-key": "secret"}
	old := keyringGet
	keyringGet = func(service, key string) (string, error) {
		if service != "Leapp" {
			t.Fatalf("service = %s", service)
		}
		v, ok := keys[key]
		if !ok {
			return "", errors.New("not found")
		}
		return v, nil
	}
	defer func() { keyringGet = old }()
	lw := &discover.LeappWorkspace{
		IAMUsers:     []discover.LeappIAMUser{{ID: "u1", Name: "personal", Region: "us-east-1"}, {ID: "u2", Name: "nokey", Region: "us-east-1"}},
		ChainedRoles: []discover.LeappChainedRole{{Name: "prod-admin", Region: "us-east-1", RoleARN: "arn:aws:iam::1:role/Admin", ParentName: "personal"}, {Name: "orphan", Region: "us-east-1", RoleARN: "arn:aws:iam::1:role/X", ParentName: "Acme/Admin"}},
	}
	res, err := s.ImportLeappSessions(lw)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Sessions) != 2 || res.Sessions[0].Name != "personal" || res.Sessions[1].Name != "prod-admin" {
		t.Fatalf("sessions = %+v", res.Sessions)
	}
	if len(res.Skipped) != 2 {
		t.Fatalf("skipped = %v", res.Skipped)
	}
	again, _ := s.ImportLeappSessions(lw)
	if len(again.Sessions) != 0 {
		t.Fatal("re-import should skip existing sessions")
	}
}

package app

import (
	"testing"

	"github.com/nateships/rolle/internal/core"
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

package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nateships/rolle/internal/aws"
	"github.com/nateships/rolle/internal/core"
)

func TestKubeTokenRefusesAWSSession(t *testing.T) {
	s := testService(t)
	sess, err := s.AddIAMUser(AddIAMUserInput{Name: "dev", Region: "us-east-1", Profile: "work", Key: aws.AccessKey{AccessKeyID: "AKIA", SecretAccessKey: "secret"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Start(context.Background(), sess.ID, StartOptions{}); err != nil {
		t.Fatal(err)
	}
	_, err = s.KubeToken(context.Background(), "dev")
	if err == nil || !strings.Contains(err.Error(), "AWS CLI") || !strings.Contains(err.Error(), `profile "work"`) {
		t.Fatalf("KubeToken on an AWS session = %v", err)
	}
	if _, err := s.KubeToken(context.Background(), "ghost"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("KubeToken unknown = %v", err)
	}
}

func TestKubeTokenNeedsAnActiveAzureSession(t *testing.T) {
	s := testService(t)
	w, _ := s.Load()
	w.Sessions = append(w.Sessions, core.Session{ID: "az1", Name: "sub", Kind: core.KindAzure, Status: core.StatusInactive, Azure: &core.AzureSession{SubscriptionID: "sub", TenantID: "t"}})
	if err := s.Save(w); err != nil {
		t.Fatal(err)
	}
	if _, err := s.KubeToken(context.Background(), "sub"); !errors.Is(err, ErrSessionInactive) {
		t.Fatalf("KubeToken on an inactive session = %v", err)
	}
	if _, _, err := s.KubeClusters(context.Background(), "sub", ""); !errors.Is(err, ErrSessionInactive) {
		t.Fatalf("KubeClusters on an inactive session = %v", err)
	}
}

func TestKubeTokenForAzureNeedsItsIntegrationAndALogin(t *testing.T) {
	s := testService(t)
	w, _ := s.Load()
	w.Sessions = append(w.Sessions, core.Session{ID: "az1", Name: "sub", Kind: core.KindAzure, Status: core.StatusActive, IntegrationID: "gone", Azure: &core.AzureSession{SubscriptionID: "sub", TenantID: "t"}})
	if err := s.Save(w); err != nil {
		t.Fatal(err)
	}
	// A session whose tenant was removed cannot mint a token.
	if _, err := s.KubeToken(context.Background(), "sub"); !errors.Is(err, core.ErrNotFound) || !strings.Contains(err.Error(), "integration for sub") {
		t.Fatalf("KubeToken without the integration = %v", err)
	}
	// With the tenant present but nobody signed in, the answer is a login.
	in, err := s.AddAzure("contoso", "7a1c2e40-3d5b-4f6a-9b8c-0d1e2f3a4b5c")
	if err != nil {
		t.Fatal(err)
	}
	w, _ = s.Load()
	w.Sessions[0].IntegrationID = in.ID
	if err := s.Save(w); err != nil {
		t.Fatal(err)
	}
	if _, err := s.KubeToken(context.Background(), "sub"); !LoginRequired(err) {
		t.Fatalf("KubeToken signed out = %v", err)
	}
}

func TestKubeTokenServesGCPFromTheCache(t *testing.T) {
	s := testService(t)
	exp := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	w, _ := s.Load()
	w.Sessions = append(w.Sessions, core.Session{ID: "g1", Name: "proj", Kind: core.KindGCP, Status: core.StatusActive, Expires: &exp, GCP: &core.GCPSession{ProjectID: "proj"}})
	if err := s.Save(w); err != nil {
		t.Fatal(err)
	}
	if err := s.Cache.Put("g1", core.Credentials{Token: "ya29.tok", Expiration: &exp}); err != nil {
		t.Fatal(err)
	}
	creds, err := s.KubeToken(context.Background(), "proj")
	if err != nil || creds.Token != "ya29.tok" || creds.Expiration == nil || !creds.Expiration.Equal(exp) {
		t.Fatalf("KubeToken = %+v, %v", creds, err)
	}
}

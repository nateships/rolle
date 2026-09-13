package app

import (
	"context"
	"errors"
	"strings"
	"testing"

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
	if _, err := s.KubeClusters(context.Background(), "sub", ""); !errors.Is(err, ErrSessionInactive) {
		t.Fatalf("KubeClusters on an inactive session = %v", err)
	}
}

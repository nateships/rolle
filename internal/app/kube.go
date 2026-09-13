package app

import (
	"context"
	"fmt"

	"github.com/nateships/rolle/internal/azure"
	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/kube"
)

// KubeToken returns the bearer token a kubectl exec plugin presents for an
// Azure or GCP session. AWS sessions authenticate through the AWS CLI and
// their profile instead.
func (s *Service) KubeToken(ctx context.Context, ref string) (core.Credentials, error) {
	w, err := s.Load()
	if err != nil {
		return core.Credentials{}, err
	}
	sess, err := FindSession(w, ref)
	if err != nil {
		return core.Credentials{}, err
	}
	switch sess.Kind.Cloud() {
	case core.CloudAWS:
		return core.Credentials{}, fmt.Errorf("%s: EKS authenticates through the AWS CLI with profile %q; rolle kube token is for Azure and Google Cloud sessions", sess.Name, ProfileName(sess))
	case core.CloudAzure:
		if sess.Status != core.StatusActive {
			return core.Credentials{}, fmt.Errorf("%s: %w", sess.Name, ErrSessionInactive)
		}
		in, err := w.Integration(sess.IntegrationID)
		if err != nil {
			return core.Credentials{}, fmt.Errorf("integration for %s: %w", sess.Name, err)
		}
		return s.azureAuth(*in).TokenFor(ctx, azure.AKSScope)
	}
	return s.credentials(ctx, w, sess)
}

// KubeClusters lists the managed clusters an active session can reach. For
// AWS, region wins over the session region, which wins over the default.
func (s *Service) KubeClusters(ctx context.Context, ref, region string) ([]kube.Cluster, error) {
	w, err := s.Load()
	if err != nil {
		return nil, err
	}
	sess, err := FindSession(w, ref)
	if err != nil {
		return nil, err
	}
	creds, err := s.credentials(ctx, w, sess)
	if err != nil {
		return nil, err
	}
	switch sess.Kind.Cloud() {
	case core.CloudAWS:
		if region == "" {
			region = sess.Region
		}
		if region == "" {
			region = w.EffectiveSettings().DefaultRegion
		}
		return kube.ListEKS(ctx, creds, region)
	case core.CloudAzure:
		return kube.ListAKS(ctx, nil, creds.Token, sess.Azure.SubscriptionID)
	case core.CloudGCP:
		return kube.ListGKE(ctx, nil, creds.Token, sess.GCP.ProjectID)
	}
	return nil, fmt.Errorf("session kind %q is not supported", sess.Kind)
}

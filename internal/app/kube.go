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

// KubeClusters lists the managed clusters an active session can reach, with
// the session ref names. For AWS, region wins over the session region, which
// wins over the default.
func (s *Service) KubeClusters(ctx context.Context, ref, region string) (*core.Session, []kube.Cluster, error) {
	w, err := s.Load()
	if err != nil {
		return nil, nil, err
	}
	sess, err := FindSession(w, ref)
	if err != nil {
		return nil, nil, err
	}
	creds, err := s.credentials(ctx, w, sess)
	if err != nil {
		return nil, nil, err
	}
	var clusters []kube.Cluster
	switch sess.Kind.Cloud() {
	case core.CloudAWS:
		if region == "" {
			region = sess.Region
		}
		if region == "" {
			region = w.EffectiveSettings().DefaultRegion
		}
		clusters, err = kube.ListEKS(ctx, creds, region)
	case core.CloudAzure:
		clusters, err = kube.ListAKS(ctx, nil, creds.Token, sess.Azure.SubscriptionID)
	case core.CloudGCP:
		clusters, err = kube.ListGKE(ctx, nil, creds.Token, sess.GCP.ProjectID)
	default:
		return nil, nil, fmt.Errorf("session kind %q is not supported", sess.Kind)
	}
	if err != nil {
		return nil, nil, err
	}
	return sess, clusters, nil
}

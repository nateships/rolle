package kube

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"go.yaml.in/yaml/v3"

	"github.com/nateships/rolle/internal/azure"
	"github.com/nateships/rolle/internal/core"
)

const aksAPIVersion = "2024-09-01"

// ListAKS returns the AKS clusters in a subscription, with the API server
// and certificate from each cluster's user kubeconfig.
func ListAKS(ctx context.Context, client *http.Client, token, subscriptionID string) ([]Cluster, error) {
	if client == nil {
		client = http.DefaultClient
	}
	var list []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Location string `json:"location"`
	}
	url := fmt.Sprintf("%s/subscriptions/%s/providers/Microsoft.ContainerService/managedClusters?api-version=%s", azure.ARMBase, subscriptionID, aksAPIVersion)
	if err := azure.ARMList(ctx, client, token, url, &list); err != nil {
		return nil, err
	}
	out := make([]Cluster, 0, len(list))
	for _, mc := range list {
		server, ca, err := aksCredentials(ctx, client, token, mc.ID)
		if err != nil {
			return nil, fmt.Errorf("aks cluster %s: %w", mc.Name, err)
		}
		out = append(out, Cluster{Name: mc.Name, Location: mc.Location, Endpoint: server, CA: ca, Cloud: core.CloudAzure})
	}
	return out, nil
}

// aksCredentials reads the server and certificate from the cluster user
// kubeconfig that ARM returns. rolle does not use the token in that kubeconfig.
func aksCredentials(ctx context.Context, client *http.Client, token, clusterID string) (string, []byte, error) {
	url := fmt.Sprintf("%s%s/listClusterUserCredential?api-version=%s", azure.ARMBase, clusterID, aksAPIVersion)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return "", nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("azure resource manager: %s: %s", resp.Status, body)
	}
	var out struct {
		Kubeconfigs []struct {
			Value []byte `json:"value"`
		} `json:"kubeconfigs"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", nil, err
	}
	if len(out.Kubeconfigs) == 0 {
		return "", nil, fmt.Errorf("no kubeconfig in the response")
	}
	var cfg struct {
		Clusters []struct {
			Cluster struct {
				Server string `yaml:"server"`
				CA     string `yaml:"certificate-authority-data"`
			} `yaml:"cluster"`
		} `yaml:"clusters"`
	}
	if err := yaml.Unmarshal(out.Kubeconfigs[0].Value, &cfg); err != nil {
		return "", nil, fmt.Errorf("parse kubeconfig: %w", err)
	}
	if len(cfg.Clusters) == 0 {
		return "", nil, fmt.Errorf("no cluster in the kubeconfig")
	}
	ca, err := base64.StdEncoding.DecodeString(cfg.Clusters[0].Cluster.CA)
	if err != nil {
		return "", nil, fmt.Errorf("certificate: %w", err)
	}
	return cfg.Clusters[0].Cluster.Server, ca, nil
}

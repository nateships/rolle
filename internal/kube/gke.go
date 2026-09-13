package kube

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/nateships/rolle/internal/core"
)

// ListGKE returns the GKE clusters of a project in every location.
func ListGKE(ctx context.Context, client *http.Client, token, project string) ([]Cluster, error) {
	if client == nil {
		client = http.DefaultClient
	}
	url := fmt.Sprintf("https://container.googleapis.com/v1/projects/%s/locations/-/clusters", project)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list gke clusters: %s: %s", resp.Status, body)
	}
	var page struct {
		Clusters []struct {
			Name       string `json:"name"`
			Location   string `json:"location"`
			Endpoint   string `json:"endpoint"`
			MasterAuth struct {
				CA string `json:"clusterCaCertificate"`
			} `json:"masterAuth"`
		} `json:"clusters"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, err
	}
	out := make([]Cluster, 0, len(page.Clusters))
	for _, c := range page.Clusters {
		ca, err := base64.StdEncoding.DecodeString(c.MasterAuth.CA)
		if err != nil {
			return nil, fmt.Errorf("gke cluster %s: certificate: %w", c.Name, err)
		}
		out = append(out, Cluster{Name: c.Name, Location: c.Location, Endpoint: "https://" + c.Endpoint, CA: ca, Cloud: core.CloudGCP, Project: project})
	}
	return out, nil
}

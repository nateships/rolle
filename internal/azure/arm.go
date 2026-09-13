// Package azure implements Entra ID sign-in and Azure Resource Manager lookups.
package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// ARMScope is the Azure Resource Manager scope requested at sign-in.
const ARMScope = "https://management.azure.com/.default"

// AKSScope is the AKS server application scope. A token for it is the bearer
// token an AKS cluster with Entra ID sign-in accepts.
const AKSScope = "6dae42f8-4368-4678-94ff-3960e28e3630/.default"

// ARMBase is the Azure Resource Manager endpoint.
const ARMBase = "https://management.azure.com"

// Subscription is one Azure subscription visible to the signed-in user.
type Subscription struct {
	ID       string `json:"subscriptionId"`
	Name     string `json:"displayName"`
	State    string `json:"state"`
	TenantID string `json:"tenantId"`
}

// ListSubscriptions returns subscriptions the token can see.
func ListSubscriptions(ctx context.Context, client *http.Client, token string) ([]Subscription, error) {
	var out []Subscription
	err := ARMList(ctx, client, token, ARMBase+"/subscriptions?api-version=2022-12-01", &out)
	return out, err
}

// ARMList follows nextLink pagination and appends "value" entries into out.
func ARMList[T any](ctx context.Context, client *http.Client, token, url string, out *[]T) error {
	if client == nil {
		client = http.DefaultClient
	}
	for url != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("azure resource manager: %s: %s", resp.Status, body)
		}
		var page struct {
			Value    []T    `json:"value"`
			NextLink string `json:"nextLink"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return err
		}
		*out = append(*out, page.Value...)
		url = page.NextLink
	}
	return nil
}

// PortalURL links to the portal for a tenant.
func PortalURL(tenantID string) string {
	if tenantID == "" {
		return "https://portal.azure.com/"
	}
	return "https://portal.azure.com/#@" + tenantID
}

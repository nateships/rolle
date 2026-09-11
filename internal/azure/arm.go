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

const armBase = "https://management.azure.com"

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
	err := armList(ctx, client, token, armBase+"/subscriptions?api-version=2022-12-01", &out)
	return out, err
}

// armList follows nextLink pagination and appends "value" entries into out.
func armList[T any](ctx context.Context, client *http.Client, token, url string, out *[]T) error {
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

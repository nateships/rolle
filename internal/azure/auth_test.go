package azure

import (
	"context"
	"testing"

	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/secrets"
)

func TestTokenWithoutLoginFails(t *testing.T) {
	a := &Auth{Integration: core.Integration{ID: "i1", Azure: &core.AzureIntegration{TenantID: "t"}}, Secrets: &secrets.Memory{}}
	if _, err := a.Token(context.Background()); err == nil {
		t.Fatal("expected login required")
	}
	if err := a.Logout(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPortalURL(t *testing.T) {
	if PortalURL("") != "https://portal.azure.com/" || PortalURL("abc") != "https://portal.azure.com/#@abc" {
		t.Fatal("unexpected portal url")
	}
}

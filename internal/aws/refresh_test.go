package aws

import (
	"errors"
	"fmt"
	"testing"

	oidctypes "github.com/aws/aws-sdk-go-v2/service/ssooidc/types"
)

func TestRefreshRejectedOnlyForOIDCRefusals(t *testing.T) {
	if !refreshRejected(fmt.Errorf("CreateToken: %w", &oidctypes.InvalidGrantException{})) {
		t.Fatal("invalid_grant must need a new login")
	}
	if !refreshRejected(&oidctypes.ExpiredTokenException{}) {
		t.Fatal("an expired refresh token must need a new login")
	}
	if refreshRejected(errors.New("dial tcp: i/o timeout")) {
		t.Fatal("a transport error must not need a new login")
	}
	if refreshRejected(&oidctypes.InternalServerException{}) {
		t.Fatal("a service fault must not need a new login")
	}
}

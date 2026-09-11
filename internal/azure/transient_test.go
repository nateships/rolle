package azure

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"testing"
)

func TestTransientSeparatesNetworkFromRefusal(t *testing.T) {
	if !transient(fmt.Errorf("server response error:\n %w", &url.Error{Op: "Post", Err: errors.New("i/o timeout")})) {
		t.Fatal("a network error is transient")
	}
	if !transient(context.DeadlineExceeded) {
		t.Fatal("a timeout is transient")
	}
	if transient(errors.New("invalid_grant: refresh token expired")) {
		t.Fatal("a refusal is not transient")
	}
}

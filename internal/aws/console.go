package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/nateships/rolle/internal/core"
)

// ConsoleURL builds a federated sign-in URL for the AWS console.
// Credentials must be temporary (have a session token).
func ConsoleURL(ctx context.Context, client *http.Client, c core.Credentials, region string) (string, error) {
	if c.SessionToken == "" {
		return "", fmt.Errorf("console sign-in needs temporary credentials")
	}
	if client == nil {
		client = http.DefaultClient
	}
	sess, err := json.Marshal(map[string]string{
		"sessionId":    c.AccessKeyID,
		"sessionKey":   c.SecretAccessKey,
		"sessionToken": c.SessionToken,
	})
	if err != nil {
		return "", err
	}
	federation := "https://signin.aws.amazon.com/federation"
	// No SessionDuration: the console session then lasts as long as the
	// credentials, and role-chained credentials are accepted.
	q := url.Values{"Action": {"getSigninToken"}, "Session": {string(sess)}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, federation+"?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("federation endpoint: %s: %s", resp.Status, body)
	}
	var tok struct {
		SigninToken string `json:"SigninToken"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", err
	}
	dest := "https://console.aws.amazon.com/"
	if region != "" {
		dest = fmt.Sprintf("https://%s.console.aws.amazon.com/console/home?region=%s", region, region)
	}
	login := url.Values{"Action": {"login"}, "Issuer": {"rolle"}, "Destination": {dest}, "SigninToken": {tok.SigninToken}}
	return federation + "?" + login.Encode(), nil
}

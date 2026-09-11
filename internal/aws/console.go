package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

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
	signinHost, consoleHost := partitionHosts(region)
	federation := "https://" + signinHost + "/federation"
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
	dest := "https://" + consoleHost + "/"
	if region != "" {
		dest = fmt.Sprintf("https://%s.%s/console/home?region=%s", region, consoleHost, region)
	}
	login := url.Values{"Action": {"login"}, "Issuer": {"rolle"}, "Destination": {dest}, "SigninToken": {tok.SigninToken}}
	return federation + "?" + login.Encode(), nil
}

// partitionHosts returns the sign-in and console hosts for the partition a
// region belongs to: GovCloud, China, or the standard partition.
func partitionHosts(region string) (signin, console string) {
	switch {
	case strings.HasPrefix(region, "us-gov-"):
		return "signin.amazonaws-us-gov.com", "console.amazonaws-us-gov.com"
	case strings.HasPrefix(region, "cn-"):
		return "signin.amazonaws.cn", "console.amazonaws.cn"
	}
	return "signin.aws.amazon.com", "console.aws.amazon.com"
}

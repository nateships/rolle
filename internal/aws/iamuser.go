package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/netcfg"
	"github.com/nateships/rolle/internal/secrets"
)

// AccessKey is a long-lived IAM user key pair.
type AccessKey struct {
	AccessKeyID     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
}

func accessKeyKey(sessionID string) string { return "aws-access-key/" + sessionID }

// ErrNoAccessKey is returned when an IAM user session has no stored key pair.
var ErrNoAccessKey = errors.New("aws iam user: no access key stored")

// StoreAccessKey saves the key pair for a session.
func StoreAccessKey(store secrets.Store, sessionID string, key AccessKey) error {
	data, err := json.Marshal(key)
	if err != nil {
		return err
	}
	return store.Set(accessKeyKey(sessionID), string(data))
}

// DeleteAccessKey removes the key pair for a session.
func DeleteAccessKey(store secrets.Store, sessionID string) error {
	return store.Delete(accessKeyKey(sessionID))
}

// LoadAccessKey reads the key pair for a session.
func LoadAccessKey(store secrets.Store, sessionID string) (AccessKey, error) {
	raw, err := store.Get(accessKeyKey(sessionID))
	if errors.Is(err, secrets.ErrNotFound) {
		return AccessKey{}, ErrNoAccessKey
	}
	if err != nil {
		return AccessKey{}, err
	}
	var k AccessKey
	if err := json.Unmarshal([]byte(raw), &k); err != nil {
		return AccessKey{}, err
	}
	return k, nil
}

// IAMUserInput describes credentials for an IAM user session.
type IAMUserInput struct {
	Key    AccessKey
	Region string
	// MFADevice and MFACode request a session token from STS when both are set.
	MFADevice string
	MFACode   string
	Duration  time.Duration
}

// IAMUserCredentials returns credentials for an IAM user. Without MFA the
// long-lived key is returned as is, with a synthetic 12 hour expiry so the
// session still rotates in the UI. With MFA, STS GetSessionToken is called.
func IAMUserCredentials(ctx context.Context, in IAMUserInput) (core.Credentials, error) {
	if in.Duration <= 0 {
		in.Duration = 12 * time.Hour
	}
	if in.MFADevice == "" || in.MFACode == "" {
		exp := time.Now().Add(in.Duration).UTC()
		return core.Credentials{AccessKeyID: in.Key.AccessKeyID, SecretAccessKey: in.Key.SecretAccessKey, Expiration: &exp}, nil
	}
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(in.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(in.Key.AccessKeyID, in.Key.SecretAccessKey, "")),
		config.WithHTTPClient(netcfg.Client()),
	)
	if err != nil {
		return core.Credentials{}, err
	}
	out, err := sts.NewFromConfig(cfg).GetSessionToken(ctx, &sts.GetSessionTokenInput{
		DurationSeconds: aws.Int32(int32(in.Duration / time.Second)),
		SerialNumber:    aws.String(in.MFADevice),
		TokenCode:       aws.String(in.MFACode),
	})
	if err != nil {
		return core.Credentials{}, fmt.Errorf("get session token: %w", err)
	}
	if out.Credentials == nil {
		return core.Credentials{}, errors.New("get session token: empty response")
	}
	return fromSTS(out.Credentials.AccessKeyId, out.Credentials.SecretAccessKey, out.Credentials.SessionToken, out.Credentials.Expiration), nil
}

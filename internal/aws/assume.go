package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/netcfg"
)

// AssumeRoleInput describes one AssumeRole call.
type AssumeRoleInput struct {
	Source      core.Credentials
	Region      string
	RoleARN     string
	SessionName string
	ExternalID  string
	// Duration defaults to one hour.
	Duration time.Duration
	// MFADevice and MFACode are passed when both are set.
	MFADevice string
	MFACode   string
}

// AssumeRole calls STS AssumeRole with the source credentials.
func AssumeRole(ctx context.Context, in AssumeRoleInput) (core.Credentials, error) {
	cfg, err := sdkConfig(in.Region, credentials.NewStaticCredentialsProvider(in.Source.AccessKeyID, in.Source.SecretAccessKey, in.Source.SessionToken))
	if err != nil {
		return core.Credentials{}, err
	}
	if in.Duration <= 0 {
		in.Duration = time.Hour
	}
	if in.SessionName == "" {
		in.SessionName = "rolle"
	}
	req := &sts.AssumeRoleInput{
		RoleArn:         aws.String(in.RoleARN),
		RoleSessionName: aws.String(in.SessionName),
		DurationSeconds: aws.Int32(int32(in.Duration / time.Second)),
	}
	if in.ExternalID != "" {
		req.ExternalId = aws.String(in.ExternalID)
	}
	if in.MFADevice != "" && in.MFACode != "" {
		req.SerialNumber = aws.String(in.MFADevice)
		req.TokenCode = aws.String(in.MFACode)
	}
	out, err := sts.NewFromConfig(cfg).AssumeRole(ctx, req)
	if err != nil {
		return core.Credentials{}, fmt.Errorf("assume role %s: %w", in.RoleARN, err)
	}
	if out.Credentials == nil {
		return core.Credentials{}, fmt.Errorf("assume role %s: empty response", in.RoleARN)
	}
	return fromSTS(out.Credentials.AccessKeyId, out.Credentials.SecretAccessKey, out.Credentials.SessionToken, out.Credentials.Expiration), nil
}

// sdkConfig builds the SDK config for one call. It does not use the SDK config
// loader, because that loader fails on a CA bundle and on a missing
// AWS_PROFILE. netcfg trusts AWS_CA_BUNDLE. Service endpoint variables in the
// environment still apply.
func sdkConfig(region string, creds aws.CredentialsProvider) (aws.Config, error) {
	env, err := config.NewEnvConfig()
	if err != nil {
		return aws.Config{}, err
	}
	return aws.Config{
		Region:        region,
		Credentials:   creds,
		HTTPClient:    netcfg.Client(),
		ConfigSources: []any{env},
	}, nil
}

func fromSTS(id, secret, token *string, exp *time.Time) core.Credentials {
	c := core.Credentials{
		AccessKeyID:     aws.ToString(id),
		SecretAccessKey: aws.ToString(secret),
		SessionToken:    aws.ToString(token),
	}
	if exp != nil {
		t := exp.UTC()
		c.Expiration = &t
	}
	return c
}

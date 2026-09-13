package kube

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/eks"

	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/netcfg"
)

// ListEKS returns the EKS clusters in region that creds can describe. optFns
// tune the client; a test points it at a fake endpoint.
func ListEKS(ctx context.Context, creds core.Credentials, region string, optFns ...func(*eks.Options)) ([]Cluster, error) {
	client := eks.NewFromConfig(aws.Config{
		Region:      region,
		Credentials: credentials.NewStaticCredentialsProvider(creds.AccessKeyID, creds.SecretAccessKey, creds.SessionToken),
		HTTPClient:  netcfg.Client(),
	}, optFns...)
	var names []string
	pages := eks.NewListClustersPaginator(client, &eks.ListClustersInput{})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list eks clusters: %w", err)
		}
		names = append(names, page.Clusters...)
	}
	out := make([]Cluster, 0, len(names))
	for _, name := range names {
		desc, err := client.DescribeCluster(ctx, &eks.DescribeClusterInput{Name: aws.String(name)})
		if err != nil {
			return nil, fmt.Errorf("describe eks cluster %s: %w", name, err)
		}
		c := Cluster{Name: name, Location: region, Cloud: core.CloudAWS, Region: region}
		if desc.Cluster != nil {
			c.Endpoint = aws.ToString(desc.Cluster.Endpoint)
			if desc.Cluster.CertificateAuthority != nil {
				if c.CA, err = base64.StdEncoding.DecodeString(aws.ToString(desc.Cluster.CertificateAuthority.Data)); err != nil {
					return nil, fmt.Errorf("eks cluster %s: certificate: %w", name, err)
				}
			}
		}
		out = append(out, c)
	}
	return out, nil
}

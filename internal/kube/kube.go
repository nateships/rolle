// Package kube discovers managed Kubernetes clusters and writes kubeconfig
// contexts that authenticate through a rolle session.
package kube

import (
	"os"
	"path/filepath"

	"github.com/nateships/rolle/internal/core"
)

// Cluster is one managed cluster a session can reach.
type Cluster struct {
	Name     string
	Location string
	Endpoint string
	// CA is the PEM certificate of the API server.
	CA    []byte
	Cloud core.Cloud
	// Region is the EKS region.
	Region string
	// ID is the AKS resource ID.
	ID string
	// Project is the GKE project.
	Project string
}

// Exec is a kubeconfig exec plugin: the command kubectl runs for a token.
type Exec struct {
	APIVersion string
	Command    string
	Args       []string
	Env        [][2]string
}

// ExecAPIVersion is the ExecCredential version rolle prints.
const ExecAPIVersion = "client.authentication.k8s.io/v1"

// AWSExec runs the AWS CLI's EKS token command with the session's profile.
func AWSExec(region, cluster, profile string) Exec {
	return Exec{
		APIVersion: "client.authentication.k8s.io/v1beta1",
		Command:    "aws",
		Args:       []string{"--region", region, "eks", "get-token", "--cluster-name", cluster, "--output", "json"},
		Env:        [][2]string{{"AWS_PROFILE", profile}},
	}
}

// RolleExec runs rolle for the session's bearer token.
func RolleExec(session string) Exec {
	return Exec{APIVersion: ExecAPIVersion, Command: "rolle", Args: []string{"kube", "token", session}}
}

// DefaultPath returns the first path in KUBECONFIG, else ~/.kube/config.
func DefaultPath() (string, error) {
	if list := filepath.SplitList(os.Getenv("KUBECONFIG")); len(list) > 0 && list[0] != "" {
		return list[0], nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".kube", "config"), nil
}
